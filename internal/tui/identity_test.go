package tui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func pendingServiceApproval(t *testing.T) (*Model, tea.Cmd, <-chan context.Context, func()) {
	t.Helper()
	workspace := t.TempDir()
	returned := make(chan context.Context, 1)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	hook := app.NewApprovalHook(nil, workspace, nil, nil)
	backend := applicationBackend{run: func(ctx context.Context, _ string) (<-chan app.BackendEvent, error) {
		events := make(chan app.BackendEvent, 2)
		go func() {
			_, err := hook(ctx, "bash", []byte(`{"command":"echo approved"}`))
			returned <- ctx
			<-release
			events <- app.BackendEvent{Text: "late"}
			events <- app.BackendEvent{Done: true, Err: err}
			close(events)
		}()
		return events, nil
	}}
	m := applicationModel(t, app.New(backend, app.Options{SessionID: "current", MaxIterations: 1}), workspace)
	t.Cleanup(func() { unblock(); m.CloseApplication() })
	cmd := m.startRun("work")
	for m.pending == nil {
		_, cmd = m.Update(cmd())
	}
	if m.pendingApprovalID == "" || m.pending.Respond != nil {
		t.Fatal("approval bypassed service broker")
	}
	return m, cmd, returned, unblock
}

func TestStaleStreamAndApprovalCannotAffectCurrentRun(t *testing.T) {
	for _, change := range []string{"session", "run", "generation"} {
		t.Run(change, func(t *testing.T) {
			m, cmd, returned, release := pendingServiceApproval(t)
			old := m.identity
			switch change {
			case "session":
				old.SessionID = "old"
			case "run":
				old.RunID--
			case "generation":
				old.Generation++
			}
			requestID := m.pendingApprovalID
			if err := m.service.RespondApproval(old, requestID, agentio.DecisionOnce); !errors.Is(err, app.ErrApprovalExpired) {
				t.Fatal("stale approval accepted", err)
			}
			obsolete := &app.Stream{}
			m.Update(applicationMsg{stream: obsolete, event: app.Event{SessionID: old.SessionID, RunID: old.RunID, Kind: "text", Text: "obsolete"}})
			outcome := agentio.RunOutcome{Status: agentio.Completed}
			m.Update(applicationMsg{stream: obsolete, outcome: &outcome})
			if !m.running || m.streamBuf.Len() != 0 || m.pendingApprovalID != requestID || m.pending == nil {
				t.Fatal("stale message mutated pending current run")
			}
			select {
			case <-returned:
				t.Fatal("stale decision released live approval")
			default:
			}
			m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
			release()
			driveApplication(t, m, cmd)
			if m.pending != nil || m.running {
				t.Fatal("valid denial did not release approval")
			}
		})
	}
}

func TestCancellationRejectsQueuedMessagesAndWaitsForJoin(t *testing.T) {
	m, cmd, returned, release := pendingServiceApproval(t)
	stream := m.activeStream
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	select {
	case ctx := <-returned:
		if ctx.Err() == nil {
			t.Fatal("backend context not cancelled")
		}
	case <-time.After(time.Second):
		t.Fatal("approval cancellation did not unblock hook")
	}
	if m.pending != nil || !m.running || m.service.Snapshot().State != app.Cancelling {
		t.Fatal("cancel did not dismiss approval and retain ownership")
	}
	m.Update(applicationMsg{stream: stream, event: app.Event{Kind: "text", Text: "late"}})
	if m.streamBuf.Len() != 0 {
		t.Fatal("cancelled run accepted queued output")
	}
	select {
	case <-stream.Terminal:
		t.Fatal("terminal before backend join")
	default:
	}
	release()
	driveApplication(t, m, cmd)
	if m.running || m.lastOutcome == nil || m.lastOutcome.Status != agentio.Cancelled || strings.Contains(strings.Join(m.transcript, "\n"), "late") {
		t.Fatal("cancelled backend completion not isolated", m.lastOutcome)
	}
	outcome, _ := stream.Wait()
	_, next := m.Update(applicationMsg{stream: stream, outcome: &outcome, event: stream.FinalEvent()})
	if next != nil {
		t.Fatal("duplicate closure scheduled more work")
	}
}

func TestSessionAndModelChangesInvalidateQueuedGoal(t *testing.T) {
	for _, command := range []string{"/new", "/model test/new"} {
		t.Run(command, func(t *testing.T) {
			service := app.New(freshAnswerBackend(), app.Options{MaxIterations: 2, Check: func(_ context.Context, _ string, iteration int) agentio.GoalLoopOutcome {
				if iteration == 1 {
					return agentio.GoalLoopOutcome{Continue: true, NextPrompt: "obsolete"}
				}
				return agentio.GoalLoopOutcome{Verified: true}
			}})
			m := applicationModel(t, service, t.TempDir())
			store := session.NewStore(t.TempDir())
			sess, err := store.Load("hand", "test")
			if err != nil {
				t.Fatal(err)
			}
			m.SetController(&Controller{Rt: &runtime.Runtime{AgentID: "hand", Session: sess, Provider: "test", Model: "old"}, Store: store, SessionKey: "test", Owner: service})
			result := captureContinuation(t, m, m.startRun("old work"))
			cmd := m.handleCommand(command)
			m.Update(result)
			if m.running {
				t.Fatal("old goal restarted after command")
			}
			if cmd != nil {
				m.Update(cmd())
			}
			m.CloseApplication()
			if err := m.controller.Rt.Session.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRunningSlashCommandCannotSwitchSessionAndQuitJoins(t *testing.T) {
	m, cmd, returned, release := pendingServiceApproval(t)
	m.handleCommand("/new")
	if !strings.Contains(strings.Join(m.transcript, "\n"), "Still working") {
		t.Fatal("running session command was not guarded")
	}
	if next := m.handleCommand("/quit"); next != nil {
		t.Fatal("quit bypassed backend cleanup")
	}
	select {
	case ctx := <-returned:
		if ctx.Err() == nil {
			t.Fatal("quit did not cancel backend")
		}
	case <-time.After(time.Second):
		t.Fatal("quit did not unblock approval")
	}
	if !m.running || m.service.Snapshot().State != app.Cancelling {
		t.Fatal("quit released ownership before join")
	}
	release()
	quit := false
	for cmd != nil {
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); ok {
			quit = true
			break
		}
		_, cmd = m.Update(msg)
	}
	if !quit || m.running || m.lastOutcome == nil || m.lastOutcome.Status != agentio.Cancelled {
		t.Fatal("quit did not follow terminal cleanup", m.lastOutcome)
	}
}
