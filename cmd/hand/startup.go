package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/runtime"
)

type startupFinished struct{ done <-chan struct{} }
type runtimeStartup struct {
	draft     textarea.Model
	failed    bool
	build     func(context.Context) (*runtime.Runtime, error)
	attempt   *app.StartupAttempt
	cancelled bool
	servers   int
}

func newRuntimeStartup(build func(context.Context) (*runtime.Runtime, error), servers int) *runtimeStartup {
	draft := textarea.New()
	draft.CharLimit = 65536
	draft.SetWidth(76)
	draft.SetHeight(4)
	draft.Placeholder = "Draft your prompt while connections initialise"
	draft.Focus()
	return &runtimeStartup{draft: draft, build: build, attempt: app.NewStartupAttempt(context.Background(), app.DefaultMCPConnectTimeout, build), servers: servers}
}
func (s *runtimeStartup) Init() tea.Cmd {
	s.attempt.Start()
	done := s.attempt.Done()
	return func() tea.Msg { <-done; return startupFinished{done} }
}
func (s *runtimeStartup) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case startupFinished:
		if msg.done != s.attempt.Done() {
			return s, nil
		}
		if s.attempt.Err() != nil && !s.cancelled {
			s.failed = true
			return s, nil
		}
		return s, tea.Quit
	case tea.WindowSizeMsg:
		s.draft.SetWidth(max(1, msg.Width-4))
		s.draft.SetHeight(max(1, min(6, msg.Height-9)))
		return s, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "esc" {
			s.cancelled = true
			s.attempt.Cancel()
			if s.failed {
				return s, tea.Quit
			}
			return s, nil
		}
		if msg.String() == "ctrl+r" && s.failed {
			_, _ = s.attempt.Finish(nil)
			s.attempt = app.NewStartupAttempt(context.Background(), app.DefaultMCPConnectTimeout, s.build)
			s.failed = false
			return s, s.Init()
		}
	}
	if s.cancelled {
		return s, nil
	}
	var cmd tea.Cmd
	s.draft, cmd = s.draft.Update(msg)
	return s, cmd
}
func (s *runtimeStartup) View() string {
	if s.cancelled {
		return "Hand — cancelling startup and closing MCP connections…\n"
	}
	state := fmt.Sprintf("Connecting %d MCP server(s)…", s.servers)
	if s.failed {
		state = "Required MCP startup failed. Ctrl+R retries; Esc quits."
	}
	return "Hand\n\n" + state + "\nRuns wait for required connections. Your draft is preserved.\n\n" + s.draft.View() + "\n\nEsc / Ctrl+C: cancel and close connections\n"

}

// finish also covers terminal disconnects before Init, and joins construction
// before the session/registry owner is released. Call after Program.Run only.
func (s *runtimeStartup) finish(programErr error) (*runtime.Runtime, error) {
	if s.cancelled {
		programErr = errors.Join(programErr, context.Canceled)
	}
	return s.attempt.Finish(programErr)
}
func buildInteractiveRuntime(construction *app.RuntimeConstruction) (*runtime.Runtime, string, error) {
	if construction.ServerCount() == 0 {
		rt, err := construction.Build(context.Background())
		return rt, "", err
	}
	startup := newRuntimeStartup(construction.Build, construction.ServerCount())
	program := tea.NewProgram(startup, tea.WithAltScreen())
	_, err := program.Run()
	rt, err := startup.finish(err)
	return rt, startup.draft.Value(), err
}
