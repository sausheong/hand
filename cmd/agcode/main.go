// Command agcode is an interactive terminal coding agent built on
// harness. Run it from the directory you want it to work in.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sausheong/agcode/internal/agentio"
	"github.com/sausheong/agcode/internal/config"
	"github.com/sausheong/agcode/internal/tui"
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
	modelFlag := flag.String("model", "", "provider/model to use, e.g. anthropic/claude-sonnet-5 (overrides ~/.agcode/config.json for this run)")
	baseURLFlag := flag.String("base-url", "", "custom API base URL, e.g. a LiteLLM proxy endpoint (overrides ~/.agcode/config.json for this run; not supported for gemini)")
	flag.Parse()

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

	sender := &programSender{}
	hook := agentio.NewApprovalHook(sender)
	spec := agentio.BuildAgentSpec(model, workspace, hook)
	reg := agentio.BuildRegistry(workspace)
	sess := session.NewSession(spec.ID, "main")

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

	m := tui.NewModel(rt)
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
		fmt.Fprintln(os.Stderr, "agcode:", err)
		os.Exit(1)
	}
}
