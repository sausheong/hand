package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSessionChangeCancellationKeepsGuardUntilWorkerJoins(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	started, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	cmd := m.startSessionChange("resume", func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-release
		return ctx.Err()
	})
	<-started
	old := m.identity
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	<-cancelled
	if !m.sessionChanging {
		t.Fatal("cancel released session guard before worker joined")
	}
	m.textarea.SetValue("must not submit")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.running {
		t.Fatal("prompt started during cancelled worker")
	}
	m.Update(sessionChangedMsg{generation: m.sessionGeneration - 1, kind: "new", id: "obsolete"})
	if !m.sessionChanging || m.identity != old {
		t.Fatal("stale result changed session")
	}
	close(release)
	m.Update(cmd())
	if m.sessionChanging || m.identity != old {
		t.Fatal("cancelled switch changed identity or retained guard")
	}
}

func TestSessionChangeShutdownJoinsWorker(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	cancelled, release := make(chan struct{}), make(chan struct{})
	m.startSessionChange("new", func(ctx context.Context) error { <-ctx.Done(); close(cancelled); <-release; return ctx.Err() })
	closed := make(chan struct{})
	go func() { m.CloseApplication(); close(closed) }()
	<-cancelled
	select {
	case <-closed:
		t.Fatal("shutdown abandoned session worker")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not join released worker")
	}
}

func TestSessionChangeCommitsResultDespiteLateCancellation(t *testing.T) {
	m, _, oldID := sessionUIFixture(t)
	cmd := m.startSessionChange("resume", func(ctx context.Context) error {
		if err := m.controller.ResumeSessionContext(ctx, oldID); err != nil {
			return err
		}
		return nil
	})
	result := cmd().(sessionChangedMsg)
	m.sessionCancel()
	m.Update(result)
	if m.identity.SessionID != oldID || m.controller.SessionID() != oldID {
		t.Fatal("late cancel split backend and UI identities")
	}
}

func TestSessionChangeFailurePreservesIdentity(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	before := m.identity
	cmd := m.startSessionChange("resume", func(context.Context) error { return errors.New("target unavailable") })
	m.Update(cmd())
	if m.identity != before || m.sessionChanging {
		t.Fatal("failed session change mutated identity")
	}
}
