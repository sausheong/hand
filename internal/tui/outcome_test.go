package tui

import (
	"context"
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	teatest "github.com/charmbracelet/x/exp/teatest"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/config"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFinalIterationStillRunsValidation(t *testing.T) {
	workspace := t.TempDir()
	checks := 0
	hooks := []config.HookConfig{{Event: "Stop", Command: "sh", Args: []string{"-c", "exit 0"}}}
	service := app.New(freshAnswerBackend(), app.Options{MaxIterations: 1, Check: func(ctx context.Context, reason string, iteration int) agentio.GoalLoopOutcome {
		checks++
		return agentio.EvaluateStopHooks(ctx, hooks, workspace, reason, iteration)
	}})
	m := applicationModel(t, service, workspace)
	driveApplication(t, m, m.startRun("verify the final iteration"))
	if checks != 1 || m.lastOutcome == nil || !m.lastOutcome.Verified || m.lastOutcome.Iterations != 1 || m.lastOutcome.Status != agentio.Completed {
		t.Fatalf("final iteration not verified: checks=%d outcome=%+v", checks, m.lastOutcome)
	}
}

func TestTUIOneTerminalOutcome(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		event        app.BackendEvent
		hooks        []config.HookConfig
		cancel       bool
		want         agentio.RunStatus
		verified     bool
	}{
		{name: "answer", reason: "completed", event: app.BackendEvent{Done: true}, want: agentio.Completed},
		{name: "final success", reason: "completed", event: app.BackendEvent{Done: true}, hooks: []config.HookConfig{{Event: "Stop", Command: "sh", Args: []string{"-c", "exit 0"}}}, want: agentio.Completed, verified: true},
		{name: "final incomplete", reason: "completed", event: app.BackendEvent{Done: true}, hooks: []config.HookConfig{{Event: "Stop", Command: "sh", Args: []string{"-c", "exit 2"}}}, want: agentio.BudgetExhausted},
		{name: "validator failure", reason: "completed", event: app.BackendEvent{Done: true}, hooks: []config.HookConfig{{Event: "Stop", Command: "sh", Args: []string{"-c", "exit 1"}}}, want: agentio.VerificationFailed},
		{name: "runtime error", reason: "error", event: app.BackendEvent{Err: errors.New("provider failed")}, want: agentio.InfrastructureError},
		{name: "turn limit", reason: "max_turns", event: app.BackendEvent{Err: errors.New("limit")}, want: agentio.BudgetExhausted},
		{name: "cancel", reason: "completed", event: app.BackendEvent{Done: true}, cancel: true, want: agentio.Cancelled},
		{name: "missing completion", reason: "error", event: app.BackendEvent{Text: "unfinished"}, want: agentio.InfrastructureError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workspace := t.TempDir()
			backend := outcomeBackend{reason: tc.reason, applicationBackend: applicationBackend{run: func(ctx context.Context, _ string) (<-chan app.BackendEvent, error) {
				events := make(chan app.BackendEvent, 1)
				if tc.cancel {
					go func() { <-ctx.Done(); events <- tc.event; close(events) }()
				} else {
					events <- tc.event
					close(events)
				}
				return events, nil
			}}}
			service := app.New(backend, app.Options{MaxIterations: 1, Check: func(ctx context.Context, reason string, iteration int) agentio.GoalLoopOutcome {
				return agentio.EvaluateStopHooks(ctx, tc.hooks, workspace, reason, iteration)
			}})
			m := applicationModel(t, service, workspace)
			cmd := m.startRun("exercise outcome")
			stream := m.activeStream
			if tc.cancel {
				m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
			}
			driveApplication(t, m, cmd)
			outcome, err := stream.Wait()
			if err != nil {
				t.Fatal(err)
			}
			duplicate := applicationMsg{stream: stream, outcome: &outcome, event: stream.FinalEvent()}
			m.Update(duplicate)
			m.Update(duplicate)
			if m.lastOutcome == nil || m.lastOutcome.Status != tc.want || m.lastOutcome.Verified != tc.verified {
				t.Fatalf("outcome %+v, want %s verified=%v", m.lastOutcome, tc.want, tc.verified)
			}
			if n := strings.Count(strings.Join(m.transcript, "\n"), "[result]"); n != 1 {
				t.Fatalf("published %d outcomes", n)
			}
		})
	}
}

type outcomeBackend struct {
	applicationBackend
	reason string
}

func (b outcomeBackend) StopReason() string { return b.reason }

func TestTUICancelledCheckCannotPublishSuccess(t *testing.T) {
	workspace := t.TempDir()
	checked, release := make(chan struct{}), make(chan struct{})
	hooks := []config.HookConfig{{Event: "Stop", Command: "sh", Args: []string{"-c", "exit 0"}}}
	service := app.New(freshAnswerBackend(), app.Options{MaxIterations: 2, Check: func(ctx context.Context, reason string, iteration int) agentio.GoalLoopOutcome {
		result := agentio.EvaluateStopHooks(ctx, hooks, workspace, reason, iteration)
		close(checked)
		<-release
		return result
	}})
	m := applicationModel(t, service, workspace)
	cmd := m.startRun("check then cancel")
	// Complete the real validator but hold its result before service commit.
	<-checked
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	close(release)
	driveApplication(t, m, cmd)
	if m.lastOutcome == nil || m.lastOutcome.Status != agentio.Cancelled || m.lastOutcome.Verified {
		t.Fatalf("outcome %+v", m.lastOutcome)
	}
	if n := strings.Count(strings.Join(m.transcript, "\n"), "[result]"); n != 1 {
		t.Fatalf("published %d outcomes", n)
	}
}

func TestInteractiveRequestPublishesCompletion(t *testing.T) {
	m := applicationModel(t, app.New(freshAnswerBackend(), app.Options{MaxIterations: 1}), t.TempDir())
	defer m.CloseApplication()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(100, 30))
	m.BindProgram(tm.GetProgram())
	tm.Type("hello")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool { return strings.Contains(string(b), "[result] Completed") }, teatest.WithDuration(2*time.Second))
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
	if m.lastOutcome == nil || m.lastOutcome.Status != agentio.Completed {
		t.Fatalf("outcome %+v", m.lastOutcome)
	}
	if n := strings.Count(strings.Join(m.transcript, "\n"), "[result]"); n != 1 {
		t.Fatalf("published %d outcomes", n)
	}
}

func freshAnswerBackend() applicationBackend {
	return applicationBackend{run: func(context.Context, string) (<-chan app.BackendEvent, error) {
		events := make(chan app.BackendEvent, 2)
		events <- app.BackendEvent{Text: "answer"}
		events <- app.BackendEvent{Done: true}
		close(events)
		return events, nil
	}}
}

func TestInteractiveContinuationPublishesOneFinalResult(t *testing.T) {
	workspace := t.TempDir()
	marker := filepath.Join(t.TempDir(), "first-check")
	hooks := []config.HookConfig{{Event: "Stop", Command: "sh", Args: []string{"-c", `if test -f "$1"; then exit 0; else : > "$1"; exit 2; fi`, "hook", marker}}}
	service := app.New(freshAnswerBackend(), app.Options{MaxIterations: 2, Check: func(ctx context.Context, reason string, iteration int) agentio.GoalLoopOutcome {
		return agentio.EvaluateStopHooks(ctx, hooks, workspace, reason, iteration)
	}})
	m := applicationModel(t, service, workspace)
	defer m.CloseApplication()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 35))
	m.BindProgram(tm.GetProgram())
	tm.Type("finish the task")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool { return strings.Contains(string(b), "configured checks passed") }, teatest.WithDuration(3*time.Second))
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
	if m.lastOutcome == nil || !m.lastOutcome.Verified || m.lastOutcome.Iterations != 2 {
		t.Fatalf("outcome %+v", m.lastOutcome)
	}
	if n := strings.Count(strings.Join(m.transcript, "\n"), "[result]"); n != 1 {
		t.Fatalf("published %d outcomes", n)
	}
}
