// Command hand is an interactive terminal coding agent built on
// harness. Run it from the directory you want it to work in.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/hand/internal/permissions"
	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/hand/internal/tui"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/providers/anthropic"
	"github.com/sausheong/harness/providers/gemini"
	"github.com/sausheong/harness/providers/openai"
	"github.com/sausheong/harness/providers/qwen"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

// version is shown in the TUI's startup banner.
const version = "0.1.0"

// programSender adapts a *tea.Program to agentio.Sender, whose method
// signature uses `any` (not bubbletea's tea.Msg) so the agentio package
// itself has no dependency on the TUI framework. Program is set after
// tea.NewProgram runs, once the *tea.Program value exists — see main().
type programSender struct {
	Program *tea.Program
}

func (s *programSender) Send(msg any) {
	s.Program.Send(msg)
}

// runOneShot drains one agent turn to stdout and returns, with no TUI —
// mirrors harness's own examples/minimal's non-streaming print loop.
// The first EventError seen becomes the process's exit error; text still
// printed before that stays on stdout (matches how the interactive path
// also shows partial output before an error).
func runOneShot(ctx context.Context, rt *runtime.Runtime, prompt string) error {
	events, err := rt.Run(ctx, prompt, nil)
	if err != nil {
		return err
	}

	var runErr error
	for ev := range events {
		switch ev.Type {
		case runtime.EventTextDelta:
			fmt.Print(ev.Text)
		case runtime.EventError:
			if runErr == nil && ev.Error != nil {
				runErr = ev.Error
			}
		}
	}
	fmt.Println()
	return runErr
}

func buildProvider(providerName, baseURL string) (llm.LLMProvider, error) {
	switch providerName {
	case "anthropic":
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set")
		}
		return anthropic.NewAnthropicProvider(key, baseURL), nil

	case "openai":
		key := os.Getenv("OPENAI_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY is not set")
		}
		return openai.NewOpenAIProvider(key, baseURL), nil

	case "gemini":
		if baseURL != "" {
			return nil, fmt.Errorf("--base-url is not supported for gemini (harness's Gemini provider has no base-URL parameter)")
		}
		key := os.Getenv("GEMINI_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY is not set")
		}
		p, err := gemini.NewGeminiProvider(context.Background(), key)
		if err != nil {
			return nil, fmt.Errorf("build gemini provider: %w", err)
		}
		return p, nil

	case "qwen":
		key := os.Getenv("DASHSCOPE_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("DASHSCOPE_API_KEY is not set")
		}
		return qwen.NewQwenProvider(key, baseURL), nil

	default:
		return nil, fmt.Errorf("unknown provider %q (want anthropic, openai, gemini, or qwen)", providerName)
	}
}

// loadTrustedPermissions loads workspace's .hand/settings.json and
// returns a permissions.Store backed by it — but if that file grants any
// always_allow entries and this workspace hasn't previously been marked
// trusted (~/.hand/trust.json), it first asks the user for confirmation.
//
// This exists because .hand/settings.json lives inside the workspace
// itself: a cloned or downloaded repo can ship one with
// {"always_allow":["bash"]} committed, which would otherwise silently
// disable hand's approval gate — including for a bash call whose
// arguments were steered by a prompt injection in that same repo's
// files — the very first time hand runs there, with no human checkpoint
// at all. Declining doesn't touch the file; its entries are just not
// honored for this run, and the user is asked again next time.
func loadTrustedPermissions(workspace string) (*permissions.Store, error) {
	settingsPath := permissions.DefaultPath(workspace)
	settings, err := permissions.Load(settingsPath)
	if err != nil {
		return nil, err
	}
	if len(settings.AlwaysAllow) == 0 {
		return permissions.NewStoreFromSettings(settingsPath, settings), nil
	}

	trustPath, err := permissions.TrustPath()
	if err != nil {
		return nil, fmt.Errorf("resolve trust store path: %w", err)
	}
	trust, err := permissions.LoadTrust(trustPath)
	if err != nil {
		return nil, fmt.Errorf("load trust store: %w", err)
	}
	if trust.IsTrusted(workspace) {
		return permissions.NewStoreFromSettings(settingsPath, settings), nil
	}

	trusted, err := promptWorkspaceTrust(settingsPath, settings.AlwaysAllow)
	if err != nil {
		return nil, fmt.Errorf("trust prompt: %w", err)
	}
	if !trusted {
		fmt.Fprintln(os.Stderr, "hand: not trusting this workspace's always-allow settings for this run; approval prompts will apply as normal")
		return permissions.NewEmptyStore(settingsPath), nil
	}
	if err := permissions.MarkTrusted(trustPath, workspace); err != nil {
		return nil, fmt.Errorf("save trust decision: %w", err)
	}
	return permissions.NewStoreFromSettings(settingsPath, settings), nil
}

// promptWorkspaceTrust asks the user, on stderr/stdin, whether to honor
// settingsPath's always-allow entries. Runs before the TUI (or one-shot
// output) exists, so a plain synchronous stdin read works for both
// modes. Any read failure — including stdin not being interactive —
// fails closed (not trusted), same as a "no" answer.
func promptWorkspaceTrust(settingsPath string, tools []string) (bool, error) {
	fmt.Fprintf(os.Stderr, "hand: %s grants always-allow (no approval prompt) for: %s\n", settingsPath, strings.Join(tools, ", "))
	fmt.Fprint(os.Stderr, "hand: trust this workspace and skip approval prompts for those tools? [y/N] ")

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

func run() error {
	modelFlag := flag.String("model", "", "provider/model to use, e.g. anthropic/claude-sonnet-5 (overrides ~/.hand/config.json for this run)")
	baseURLFlag := flag.String("base-url", "", "custom API base URL, e.g. a LiteLLM proxy endpoint (overrides ~/.hand/config.json for this run; not supported for gemini)")
	maxTurnsFlag := flag.Int("max-turns", 0, "cap the agent's tool-use loop for this run (overrides ~/.hand/config.json for this run; 0 means use the config/default)")
	fallbackModelFlag := flag.String("fallback-model", "", "provider/model to retry against on a transient provider error, same provider as --model (overrides ~/.hand/config.json for this run)")
	newSessionFlag := flag.Bool("new-session", false, "discard this workspace's saved session and start fresh")
	printFlag := flag.String("p", "", "run one turn non-interactively with this prompt, print the result, and exit (no TUI)")
	yesFlag := flag.Bool("yes", false, "auto-approve all gated tool calls for this run (only valid with -p)")
	flag.Parse()

	oneShot := *printFlag != ""
	if *yesFlag && !oneShot {
		return fmt.Errorf("--yes only applies together with -p")
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))

	cfgPath, err := config.DefaultPath()
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	model := config.ResolveModel(*modelFlag, cfg)
	baseURL := config.ResolveBaseURL(*baseURLFlag, cfg)
	maxTurns := config.ResolveMaxTurns(*maxTurnsFlag, cfg)
	fallbackModel := config.ResolveFallbackModel(*fallbackModelFlag, cfg)

	providerName, bareModel := llm.ParseProviderModel(model)
	if providerName == "" {
		return fmt.Errorf("model %q must be in \"provider/model\" form, e.g. anthropic/claude-sonnet-5", model)
	}
	provider, err := buildProvider(providerName, baseURL)
	if err != nil {
		return fmt.Errorf("build provider %q: %w", providerName, err)
	}

	workspace, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}

	perms, err := loadTrustedPermissions(workspace)
	if err != nil {
		return fmt.Errorf("load permissions: %w", err)
	}

	mcpServers := cfg.ToServerConfigs()
	allMCPServerNames := cfg.AllMCPServerNames()
	trustedServers := cfg.TrustedMCPServers()

	var sender *programSender
	var hook func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error)
	if oneShot {
		hook = agentio.NewOneShotApprovalHook(perms, *yesFlag, allMCPServerNames, trustedServers)
	} else {
		sender = &programSender{}
		hook = agentio.NewApprovalHook(sender, perms, workspace, allMCPServerNames, trustedServers)
	}
	spec := agentio.BuildAgentSpec(model, workspace, maxTurns, fallbackModel, mcpServers, hook)
	// reg must stay the concrete *tool.Registry type below (RuntimeInputs.Tools
	// is the wider tool.Executor interface) — BuildRuntime type-asserts it to
	// register spec.MCPServers' tools and silently skips registration
	// otherwise, per harness's own comment in runtime/builder.go.
	reg := agentio.BuildRegistry(workspace)
	compactionMgr := agentio.BuildCompactionManager(provider, bareModel)

	storeDir, err := sessionio.StoreDir()
	if err != nil {
		return fmt.Errorf("resolve session store directory: %w", err)
	}
	store := session.NewStore(storeDir)
	sessionKey := sessionio.KeyForWorkspace(workspace)
	if *newSessionFlag {
		if err := store.Delete(spec.ID, sessionKey); err != nil {
			return fmt.Errorf("discard previous session: %w", err)
		}
	}
	sess, err := store.Load(spec.ID, sessionKey)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	// BuildRuntimeWithTimeout connects every configured MCP server
	// synchronously, before the TUI (or one-shot output) exists — a slow
	// server adds real, silent delay to startup otherwise. This is the
	// only feedback the user gets until it either succeeds or the
	// timeout fires; a fuller fix would start the TUI first and connect
	// in the background, but that needs Runner to be swappable after
	// construction, a larger change than this warrants right now.
	if len(mcpServers) > 0 {
		fmt.Fprintf(os.Stderr, "hand: connecting to %d configured MCP server(s)...\n", len(mcpServers))
	}
	rt, err := agentio.BuildRuntimeWithTimeout(
		runtime.RuntimeDeps{},
		runtime.RuntimeInputs{
			Provider:   provider,
			Tools:      reg,
			Session:    sess,
			Compaction: compactionMgr,
		},
		spec,
		agentio.DefaultMCPConnectTimeout,
	)
	if err != nil {
		return fmt.Errorf("build runtime: %w", err)
	}
	defer rt.Close()

	if oneShot {
		return runOneShot(context.Background(), rt, *printFlag)
	}

	m := tui.NewModel(rt, workspace)
	m.SetBanner(version, model, workspace)
	m.SetController(&tui.Controller{
		Rt:            rt,
		Store:         store,
		SessionKey:    sessionKey,
		BaseURL:       baseURL,
		BuildProvider: buildProvider,
	})
	if history := sess.History(); len(history) > 0 {
		m.LoadHistory(history)
	}
	program := tea.NewProgram(m, tea.WithAltScreen())
	m.BindProgram(program)
	sender.Program = program

	if _, err := program.Run(); err != nil {
		return fmt.Errorf("run TUI: %w", err)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "hand:", err)
		os.Exit(1)
	}
}
