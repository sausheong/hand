package tui

import (
	"context"
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/config"
	"testing"
	"time"
)

func TestCancelledGoalResultCannotContinue(t *testing.T) {
	started := make(chan struct{})
	calls := 0
	backend := applicationBackend{run: func(ctx context.Context, _ string) (<-chan app.BackendEvent, error) {
		calls++
		events := make(chan app.BackendEvent)
		go func() { close(started); <-ctx.Done(); close(events) }()
		return events, nil
	}}
	service := app.New(backend, app.Options{MaxIterations: 2})
	m := applicationModel(t, service, t.TempDir())
	cmd := m.startRun("cancel")
	old := m.activeStream
	<-started
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	driveApplication(t, m, cmd)
	before := len(m.transcript)
	m.Update(applicationMsg{stream: old, event: app.Event{Kind: "continuation", Iteration: 2, Text: "obsolete"}})
	success := agentio.RunOutcome{Status: agentio.Completed, Verified: true}
	m.Update(applicationMsg{stream: old, outcome: &success})
	if calls != 1 || m.running || m.lastOutcome == nil || m.lastOutcome.Status != agentio.Cancelled || len(m.transcript) != before {
		t.Fatal("cancelled stream changed the settled run", calls, m.lastOutcome)
	}
}

func TestSupersededGoalCheckCannotContinue(t *testing.T) {
	for _, action := range []string{"cancel", "new input", "new check"} {
		t.Run(action, func(t *testing.T) {
			workspace := t.TempDir()
			checked, release := make(chan struct{}), make(chan struct{})
			calls := 0
			backend := applicationBackend{run: func(context.Context, string) (<-chan app.BackendEvent, error) {
				calls++
				events := make(chan app.BackendEvent, 1)
				events <- app.BackendEvent{Done: true}
				close(events)
				return events, nil
			}}
			hooks := []config.HookConfig{{Event: "Stop", Command: "sh", Args: []string{"-c", "echo obsolete; exit 2"}}}
			service := app.New(backend, app.Options{MaxIterations: 10, Check: func(ctx context.Context, reason string, iteration int) agentio.GoalLoopOutcome {
				result := agentio.EvaluateStopHooks(ctx, hooks, workspace, reason, iteration)
				close(checked)
				<-release
				return result
			}})
			m := applicationModel(t, service, workspace)
			cmd := m.startRun("old work")
			for !m.goalChecking {
				_, cmd = m.Update(cmd())
			}
			<-checked
			switch action {
			case "new input":
				if m.startRun("new work") != nil {
					t.Fatal("new input bypassed active checker")
				}
			case "new check":
				if _, err := service.Start(context.Background(), "new check", nil); !errors.Is(err, app.ErrBusy) {
					t.Fatal("overlapping check admitted", err)
				}
			}
			m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
			close(release)
			driveApplication(t, m, cmd)
			if calls != 1 || m.lastOutcome == nil || m.lastOutcome.Status != agentio.Cancelled {
				t.Fatal("obsolete check continued", calls, m.lastOutcome)
			}
		})
	}
}

func TestCancelGoalCheckCancelsContextAndInvalidatesResults(t *testing.T) {
	entered, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	service := app.New(freshAnswerBackend(), app.Options{MaxIterations: 2, Check: func(ctx context.Context, _ string, _ int) agentio.GoalLoopOutcome {
		close(entered)
		<-ctx.Done()
		close(cancelled)
		<-release
		return agentio.GoalLoopOutcome{Continue: true, NextPrompt: "obsolete"}
	}})
	m := applicationModel(t, service, t.TempDir())
	cmd := m.startRun("work")
	for !m.goalChecking {
		_, cmd = m.Update(cmd())
	}
	<-entered
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("checker context not cancelled")
	}
	if !m.running || service.Snapshot().State != app.Cancelling {
		t.Fatal("cancellation released checker ownership before join")
	}
	close(release)
	driveApplication(t, m, cmd)
	if m.goalChecking || m.lastOutcome == nil || m.lastOutcome.Status != agentio.Cancelled || m.lastOutcome.Iterations != 1 {
		t.Fatal("cancelled check continued", m.lastOutcome)
	}
}

func TestGoalResultConsumedOnlyOnce(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	calls := 0
	backend := applicationBackend{run: func(_ context.Context, prompt string) (<-chan app.BackendEvent, error) {
		calls++
		events := make(chan app.BackendEvent, 1)
		if calls == 1 {
			events <- app.BackendEvent{Done: true}
			close(events)
		} else {
			if prompt != "next" {
				t.Error("wrong continuation prompt", prompt)
			}
			go func() { close(started); <-release; events <- app.BackendEvent{Done: true}; close(events) }()
		}
		return events, nil
	}}
	service := app.New(backend, app.Options{MaxIterations: 2, Check: func(_ context.Context, _ string, iteration int) agentio.GoalLoopOutcome {
		if iteration == 1 {
			return agentio.GoalLoopOutcome{Continue: true, NextPrompt: "next"}
		}
		return agentio.GoalLoopOutcome{Verified: true}
	}})
	m := applicationModel(t, service, t.TempDir())
	cmd := m.startRun("initial")
	for {
		msg := cmd()
		_, cmd = m.Update(msg)
		if event, ok := msg.(applicationMsg); ok && event.event.Kind == "continuation" {
			m.Update(event)
			break
		}
	}
	<-started
	if calls != 2 {
		t.Fatal("duplicate continuation started another backend turn", calls)
	}
	close(release)
	driveApplication(t, m, cmd)
	if calls != 2 || m.lastOutcome == nil || m.lastOutcome.Iterations != 2 || !m.lastOutcome.Verified {
		t.Fatal(calls, m.lastOutcome)
	}
}

func TestEventDoneDoesNotReleaseRuntimeBeforeChannelCloses(t *testing.T) {
	release := make(chan struct{})
	backend := applicationBackend{run: func(context.Context, string) (<-chan app.BackendEvent, error) {
		events := make(chan app.BackendEvent)
		go func() {
			events <- app.BackendEvent{Kind: "usage", Done: true}
			<-release
			close(events)
		}()
		return events, nil
	}}
	service := app.New(backend, app.Options{MaxIterations: 1})
	m := applicationModel(t, service, t.TempDir())
	cmd := m.startRun("work")
	stream := m.activeStream
	// Read through the done event while cleanup deliberately retains the channel.
	for {
		msg := cmd()
		_, cmd = m.Update(msg)
		if event, ok := msg.(applicationMsg); ok && event.event.Kind == "usage" {
			break
		}
	}
	if !m.running || m.lastOutcome != nil || service.Snapshot().State != app.Running {
		t.Fatal("done released run before cleanup")
	}
	if _, err := service.Start(context.Background(), "overlap", nil); !errors.Is(err, app.ErrBusy) {
		t.Fatal("overlapping run admitted", err)
	}
	select {
	case <-stream.Terminal:
		t.Fatal("terminal preceded cleanup")
	default:
	}
	close(release)
	driveApplication(t, m, cmd)
	if m.running || m.lastOutcome == nil || m.lastOutcome.Status != agentio.Completed {
		t.Fatal(m.lastOutcome)
	}
}

// captureContinuation joins the goal while retaining an actual delivered
// continuation event for delayed-delivery regression checks.
func captureContinuation(t *testing.T, m *Model, cmd tea.Cmd) applicationMsg {
	t.Helper()
	var captured applicationMsg
	for cmd != nil {
		msg := cmd()
		if application, ok := msg.(applicationMsg); ok {
			for _, event := range append([]app.Event{application.event}, application.following...) {
				if event.Kind == "continuation" {
					captured = applicationMsg{stream: application.stream, event: event}
				}
			}
		}
		_, cmd = m.Update(msg)
	}
	if captured.stream == nil {
		t.Fatal("service did not publish continuation")
	}
	return captured
}

func TestNewSessionRejectsQueuedGoalContinuation(t *testing.T) {
	fixture, _, _ := sessionUIFixture(t)
	controller := fixture.controller
	oldID := controller.SessionID()
	workspace := t.TempDir()
	calls := 0
	backend := applicationBackend{run: func(context.Context, string) (<-chan app.BackendEvent, error) {
		calls++
		events := make(chan app.BackendEvent, 1)
		events <- app.BackendEvent{Done: true}
		close(events)
		return events, nil
	}}
	hooks := []config.HookConfig{{Event: "Stop", Command: "sh", Args: []string{"-c", "echo obsolete; exit 2"}}}
	controller.Owner = app.New(backend, app.Options{SessionID: oldID, MaxIterations: 2, Check: func(ctx context.Context, reason string, iteration int) agentio.GoalLoopOutcome {
		return agentio.EvaluateStopHooks(ctx, hooks, workspace, reason, iteration)
	}})
	m := applicationModel(t, controller.Owner, workspace)
	m.SetController(controller)
	defer m.CloseApplication()
	result := captureContinuation(t, m, m.startRun("old session work"))
	if calls != 2 || result.event.SessionID != oldID || result.event.Iteration != 2 {
		t.Fatal("invalid captured continuation", calls, result.event)
	}
	change := m.handleCommand("/new")
	if change == nil {
		t.Fatal("session change not scheduled")
	}
	m.Update(result)
	if m.running {
		t.Fatal("queued old goal started during session change")
	}
	m.Update(change())
	if m.controller.SessionID() == oldID || m.sessionChanging {
		t.Fatal("new session did not commit")
	}
	m.Update(result)
	if m.running || m.goalChecking || calls != 2 {
		t.Fatal("queued old goal started in new session")
	}
	if entries := m.controller.SessionHistory(); len(entries) != 0 {
		t.Fatalf("old continuation contaminated new session: %v", entries)
	}
}
