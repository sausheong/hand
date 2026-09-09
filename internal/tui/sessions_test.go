package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func sessionUIFixture(t *testing.T) (*Model, *sessionio.Manager, string) {
	t.Helper()
	manager, err := sessionio.NewManager(t.TempDir(), t.TempDir(), "hand")
	if err != nil {
		t.Fatal(err)
	}
	old, err := manager.Open(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	old.Session.Append(session.UserMessageEntry("original transcript"))
	if err := old.Session.Flush(); err != nil {
		t.Fatal(err)
	}
	old.Session.Close()
	current, err := manager.Open(context.Background(), "", true)
	if err != nil {
		t.Fatal(err)
	}
	current.Session.Append(session.UserMessageEntry("current transcript"))
	controller := &Controller{Rt: &runtime.Runtime{AgentID: "hand", Session: current.Session}, Sessions: manager, SessionKey: current.Record.StoreKey}
	m := NewModel(nil, t.TempDir())
	m.SetController(controller)
	m.LoadHistory(current.Session.History())
	t.Cleanup(func() { controller.Rt.Session.Close() })
	return m, manager, old.Session.ID
}

func TestSessionCommandsListNameAndResume(t *testing.T) {
	m, manager, oldID := sessionUIFixture(t)
	m.Update(m.handleCommand("/name My  current notes")())
	snapshot, err := manager.Catalogue().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, record := range snapshot.Sessions {
		if record.ID == m.controller.SessionID() {
			found = record.Name == "My  current notes"
		}
	}
	if !found {
		t.Fatal("name not persisted exactly")
	}
	m.Update(m.handleCommand("/resume")())
	if !strings.Contains(strings.Join(m.transcript, "\n"), oldID) {
		t.Fatal("saved session not listed")
	}
	before := m.identity
	m.lastUsage = &llm.Usage{InputTokens: 99}
	m.sessionUsage = llm.Usage{InputTokens: 99}
	m.toolCallsThisTurn = 4
	if cmd := m.handleCommand("/resume " + oldID); cmd != nil {
		m.Update(cmd())
	}
	text := strings.Join(m.transcript, "\n")
	if m.identity.SessionID != oldID || m.identity.Generation <= before.Generation || !strings.Contains(text, "original transcript") || strings.Contains(text, "current transcript") {
		t.Fatal("resume did not replace transcript/identity", text)
	}
	if m.lastUsage != nil || m.sessionUsage.InputTokens != 0 || m.toolCallsThisTurn != 0 {
		t.Fatal("prior session counters leaked")
	}
	m.Update(applicationMsg{stream: &app.Stream{}, event: app.Event{SessionID: before.SessionID, RunID: before.RunID, Kind: "text", Text: "stale text"}})
	if strings.Contains(strings.Join(m.transcript, "\n"), "stale text") || m.streamBuf.Len() != 0 {
		t.Fatal("stale event changed resumed view")
	}
}

func TestSessionCommandsPreserveViewOnFailureAndRejectActiveRun(t *testing.T) {
	m, manager, oldID := sessionUIFixture(t)
	current := m.controller.SessionID()
	other, err := manager.Open(context.Background(), oldID, false)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Session.Close()
	if cmd := m.handleCommand("/resume " + oldID); cmd != nil {
		m.Update(cmd())
	}
	if m.controller.SessionID() != current || !strings.Contains(strings.Join(m.transcript, "\n"), "current transcript") {
		t.Fatal("busy target replaced current session")
	}
	m.running = true
	if cmd := m.handleCommand("/resume " + oldID); cmd != nil {
		m.Update(cmd())
	}
	m.handleCommand("/name forbidden")
	if m.controller.SessionID() != current {
		t.Fatal("active run switched session")
	}
	snapshot, err := manager.Catalogue().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range snapshot.Sessions {
		if record.Name == "forbidden" {
			t.Fatal("active run renamed session")
		}
	}
}

func TestSessionMetadataPreservesViewAndCounters(t *testing.T) {
	for _, command := range []string{"/resume", "/name retained"} {
		t.Run(command, func(t *testing.T) {
			m, _, _ := sessionUIFixture(t)
			before := m.identity
			m.sessionUsage = llm.Usage{InputTokens: 77}
			m.toolCallsThisTurn = 3
			cmd := m.handleCommand(command)
			if cmd == nil || !m.sessionChanging {
				t.Fatal("metadata did not launch guarded worker")
			}
			m.Update(cmd())
			if m.identity != before || m.sessionUsage.InputTokens != 77 || m.toolCallsThisTurn != 3 || !strings.Contains(strings.Join(m.transcript, "\n"), "current transcript") {
				t.Fatal("metadata operation replaced session state")
			}
		})
	}
}

func TestSessionMetadataCancelledContextDoesNotRename(t *testing.T) {
	m, manager, _ := sessionUIFixture(t)
	before, err := manager.Catalogue().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.controller.RenameSessionContext(ctx, "forbidden"); err != context.Canceled {
		t.Fatalf("rename: %v", err)
	}
	if _, err := m.controller.ListSessionsContext(ctx); err != context.Canceled {
		t.Fatalf("list: %v", err)
	}
	after, err := manager.Catalogue().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if before.Generation != after.Generation {
		t.Fatal("cancelled operation mutated catalogue")
	}
}
