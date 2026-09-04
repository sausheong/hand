// Command hand is an interactive terminal coding agent built on
// harness. Run it from the directory you want it to work in.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"

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

func run() error {
	modelFlag := flag.String("model", "", "provider/model to use, e.g. anthropic/claude-sonnet-5 (overrides ~/.hand/config.json for this run)")
	baseURLFlag := flag.String("base-url", "", "custom API base URL, e.g. a LiteLLM proxy endpoint (overrides ~/.hand/config.json for this run; not supported for gemini)")
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

	providerName, _ := llm.ParseProviderModel(model)
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

	perms, err := permissions.NewStore(permissions.DefaultPath(workspace))
	if err != nil {
		return fmt.Errorf("load permissions: %w", err)
	}

	var sender *programSender
	var hook func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error)
	if oneShot {
		hook = agentio.NewOneShotApprovalHook(perms, *yesFlag)
	} else {
		sender = &programSender{}
		hook = agentio.NewApprovalHook(sender, perms, workspace)
	}
	spec := agentio.BuildAgentSpec(model, workspace, hook)
	reg := agentio.BuildRegistry(workspace)

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

	rt, err := runtime.BuildRuntime(
		runtime.RuntimeDeps{},
		runtime.RuntimeInputs{
			Provider: provider,
			Tools:    reg,
			Session:  sess,
		},
		spec,
	)
	if err != nil {
		return fmt.Errorf("build runtime: %w", err)
	}
	defer rt.Close()

	if oneShot {
		return runOneShot(context.Background(), rt, *printFlag)
	}

	m := tui.NewModel(rt)
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
