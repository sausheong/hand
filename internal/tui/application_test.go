package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/llm"
)

type applicationBackend struct {
	run func(context.Context, string) (<-chan app.BackendEvent, error)
}

func applicationModel(t testing.TB, service *app.Service, workspace string) *Model {
	t.Helper()
	m, err := NewApplicationModel(service, workspace)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// NewModel initializes presentation fixtures with the production constructor.
func NewModel(_ any, workspace string) *Model {
	m, err := NewApplicationModel(app.New(nil, app.Options{MaxIterations: 1}), workspace)
	if err != nil {
		panic(err)
	}
	return m
}

func (b applicationBackend) Run(ctx context.Context, p string, _ []llm.ImageContent) (<-chan app.BackendEvent, error) {
	return b.run(ctx, p)
}
func (b applicationBackend) StopReason() string { return "" }
func driveApplication(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for cmd != nil {
		result := make(chan tea.Msg, 1)
		go func(c tea.Cmd) { result <- c() }(cmd)
		select {
		case msg := <-result:
			_, cmd = m.Update(msg)
		case <-deadline:
			t.Fatal("application delivery did not finish")
		}
	}
}

func TestApplicationTUIUsesSharedGoalChecksAndUsage(t *testing.T) {
	var prompts []string
	backend := applicationBackend{run: func(ctx context.Context, p string) (<-chan app.BackendEvent, error) {
		prompts = append(prompts, p)
		if agentio.IdentityFromContext(ctx).SessionID != "session" {
			t.Error("missing service identity")
		}
		ch := make(chan app.BackendEvent, 4)
		ch <- app.BackendEvent{Kind: "tool_call", Details: app.Details{ToolPresent: true, ToolName: "read_file", ToolInput: `{"path":"file"}`}}
		ch <- app.BackendEvent{Kind: "tool_result", Details: app.Details{Output: "contents"}}
		ch <- app.BackendEvent{Text: "answer"}
		ch <- app.BackendEvent{Kind: "usage", Done: true, Details: app.Details{UsageKnown: true, InputTokens: 10, OutputTokens: 2}}
		close(ch)
		return ch, nil
	}}
	checks := 0
	service := app.New(backend, app.Options{SessionID: "session", MaxIterations: 2, Check: func(context.Context, string, int) agentio.GoalLoopOutcome {
		checks++
		if checks == 1 {
			return agentio.GoalLoopOutcome{Continue: true, NextPrompt: "fix"}
		}
		return agentio.GoalLoopOutcome{Verified: true}
	}})
	m, err := NewApplicationModel(service, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cmd := m.startRun("initial")
	if m.identity != m.activeStream.Identity() {
		t.Fatal("UI identity differs from service")
	}
	driveApplication(t, m, cmd)
	if m.running || m.goalChecking || m.lastOutcome == nil || m.lastOutcome.Status != agentio.Completed || !m.lastOutcome.Verified || m.lastOutcome.Iterations != 2 || checks != 2 || len(prompts) != 2 || prompts[1] != "fix" {
		t.Fatalf("outcome=%+v checks=%d prompts=%v", m.lastOutcome, checks, prompts)
	}
	text := strings.Join(m.transcript, "\n")
	if strings.Count(text, "[result]") != 1 || !strings.Contains(text, "read_file") || !strings.Contains(text, "contents") || !strings.Contains(text, "continuing") || m.sessionUsage.InputTokens != 20 {
		t.Fatal(text, m.sessionUsage)
	}
}

func TestApplicationConstructorRejectsMissingService(t *testing.T) {
	m, err := NewApplicationModel(nil, t.TempDir())
	if err == nil || m != nil {
		t.Fatalf("missing service accepted: model=%v error=%v", m, err)
	}
}

func TestApplicationTUICancellationWaitsForCheckerAndRejectsStaleEvents(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	backend := applicationBackend{run: func(context.Context, string) (<-chan app.BackendEvent, error) {
		ch := make(chan app.BackendEvent, 1)
		ch <- app.BackendEvent{Done: true}
		close(ch)
		return ch, nil
	}}
	service := app.New(backend, app.Options{SessionID: "session", MaxIterations: 2, Check: func(ctx context.Context, _ string, _ int) agentio.GoalLoopOutcome {
		close(started)
		<-release
		return agentio.GoalLoopOutcome{Continue: true, NextPrompt: "must not run"}
	}})
	m := applicationModel(t, service, t.TempDir())
	cmd := m.startRun("work")
	// Drain up to the checking state; the worker remains deliberately held.
	for !m.goalChecking {
		_, cmd = m.Update(cmd())
	}
	<-started
	old := m.activeStream
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !m.running || m.lastOutcome != nil || service.Snapshot().State != app.Cancelling {
		t.Fatal("cancel released ownership early")
	}
	if m.startRun("overlap") != nil {
		t.Fatal("overlapping run started")
	}
	close(release)
	driveApplication(t, m, cmd)
	if m.lastOutcome == nil || m.lastOutcome.Status != agentio.Cancelled || m.lastOutcome.Iterations != 1 {
		t.Fatal(m.lastOutcome)
	}
	before := len(m.transcript)
	m.Update(applicationMsg{stream: old, event: app.Event{Kind: "text", Text: "obsolete"}})
	if len(m.transcript) != before || strings.Contains(m.streamBuf.String(), "obsolete") {
		t.Fatal("stale application event rendered")
	}
}

func TestApplicationTUICloseCancelsAndJoinsUnreadStream(t *testing.T) {
	joined := make(chan struct{})
	backend := applicationBackend{run: func(ctx context.Context, _ string) (<-chan app.BackendEvent, error) {
		ch := make(chan app.BackendEvent)
		go func() { <-ctx.Done(); close(joined); close(ch) }()
		return ch, nil
	}}
	m := applicationModel(t, app.New(backend, app.Options{MaxIterations: 1}), t.TempDir())
	cmd := m.startRun("work")
	// Receive Running before closing so the backend has been started.
	m.Update(cmd())
	m.CloseApplication()
	select {
	case <-joined:
	case <-time.After(time.Second):
		t.Fatal("backend not joined")
	}
}

func TestApplicationTUIApprovalUsesServiceIdentity(t *testing.T) {
	hook := app.NewApprovalHook(nil, t.TempDir(), nil, nil)
	allowed := make(chan bool, 1)
	backend := applicationBackend{run: func(ctx context.Context, _ string) (<-chan app.BackendEvent, error) {
		ch := make(chan app.BackendEvent, 1)
		go func() {
			decision, err := hook(ctx, "bash", []byte(`{"command":"echo approved"}`))
			allowed <- decision.Allow && err == nil
			ch <- app.BackendEvent{Done: true}
			close(ch)
		}()
		return ch, nil
	}}
	m := applicationModel(t, app.New(backend, app.Options{SessionID: "approval-session", MaxIterations: 1}), t.TempDir())
	cmd := m.startRun("work")
	for m.pending == nil {
		_, cmd = m.Update(cmd())
	}
	if m.pending.Identity != m.identity || m.pendingApprovalID == "" || m.pending.Respond != nil {
		t.Fatal("approval identity/transport differs from service")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	driveApplication(t, m, cmd)
	if !<-allowed || m.lastOutcome.Status != agentio.Completed {
		t.Fatal("approval not delivered")
	}
}

func TestApplicationOwnedApprovalJourney(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		name := "approve"
		if cancel {
			name = "cancel"
		}
		t.Run(name, func(t *testing.T) {
			hook := app.NewApprovalHook(nil, t.TempDir(), nil, nil)
			allowed := make(chan bool, 1)
			backend := applicationBackend{run: func(ctx context.Context, _ string) (<-chan app.BackendEvent, error) {
				ch := make(chan app.BackendEvent, 1)
				go func() {
					decision, err := hook(ctx, "bash", []byte(`{"command":"echo approved"}`))
					allowed <- decision.Allow
					ch <- app.BackendEvent{Done: true, Err: err}
					close(ch)
				}()
				return ch, nil
			}}
			m := applicationModel(t, app.New(backend, app.Options{SessionID: "owned-approval", MaxIterations: 1}), t.TempDir())
			cmd := m.startRun("work")
			for m.pending == nil {
				_, cmd = m.Update(cmd())
			}
			if m.pendingApprovalID == "" || m.pending.Respond != nil || m.service.Snapshot().State != app.AwaitingApproval {
				t.Fatal("approval still uses legacy transport")
			}
			if cancel {
				m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
			} else {
				m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
			}
			driveApplication(t, m, cmd)
			if <-allowed == cancel || m.pending != nil || m.running {
				t.Fatal("incorrect approval completion")
			}
			expected := agentio.Completed
			if cancel {
				expected = agentio.Cancelled
			}
			if m.lastOutcome.Status != expected {
				t.Fatal(m.lastOutcome)
			}
		})
	}
}
