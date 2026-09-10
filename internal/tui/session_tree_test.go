package tui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sausheong/harness/session"
)

func TestSessionTreeSelectionPersistsAcrossRestart(t *testing.T) {
	m, manager, _ := sessionUIFixture(t)
	sess := m.controller.Rt.Session
	root := sess.LeafID()
	sess.Append(session.UserMessageEntry("later branch"))
	later := sess.LeafID()
	before := m.identity
	m.Update(m.handleCommand("/tree")())
	display := strings.Join(m.transcript, "\n")
	if !strings.Contains(display, root) || !strings.Contains(display, "* "+later) {
		t.Fatal("tree omitted topology/selection", display)
	}
	if m.identity != before {
		t.Fatal("listing changed identity")
	}
	m.Update(m.handleCommand("/tree " + root)())
	if sess.LeafID() != root || m.identity.Generation <= before.Generation {
		t.Fatal("node selection did not update identity/leaf")
	}
	if strings.Contains(strings.Join(m.transcript, "\n"), "later branch") {
		t.Fatal("selected replay included other branch")
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := manager.Open(context.Background(), sess.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	m.controller.Rt.Session = reopened.Session
	if reopened.Session.LeafID() != root {
		t.Fatal("selection lost on restart")
	}
	nodes, err := m.controller.SessionTree(context.Background())
	if err != nil || len(nodes) != 2 {
		t.Fatal("tree lost other branch or exposed control record", nodes, err)
	}
	if err := m.controller.SelectSessionNode(context.Background(), later); err != nil {
		t.Fatal(err)
	}
	if len(reopened.Session.History()) != 2 {
		t.Fatal("original branch inaccessible")
	}
}

func TestSessionTreeInvalidCancelledAndActiveSelection(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	sess := m.controller.Rt.Session
	leaf := sess.LeafID()
	before := m.identity
	m.Update(m.handleCommand("/tree missing")())
	if sess.LeafID() != leaf || m.identity != before {
		t.Fatal("failed selection changed view")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.controller.SelectSessionNode(ctx, leaf); err != context.Canceled {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := m.controller.SessionTree(ctx); err != context.Canceled {
		t.Fatalf("cancelled tree: %v", err)
	}
	count := len(sess.Entries())
	m.running = true
	if cmd := m.handleCommand("/tree " + leaf); cmd != nil {
		t.Fatal("active command launched")
	}
	if len(sess.Entries()) != count {
		t.Fatal("active command persisted a selection")
	}
}

func TestSessionTreeExcludesDurableAnnotations(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	leaf := m.controller.Rt.Session.LeafID()
	if err := m.controller.Rt.Session.Annotate("hand.usage", json.RawMessage(`{"request_id":"one"}`)); err != nil {
		t.Fatal(err)
	}
	nodes, err := m.controller.SessionTree(context.Background())
	if err != nil || len(nodes) != 1 || nodes[0].ID != leaf || !nodes[0].Selected {
		t.Fatal("annotation exposed as conversation", nodes, err)
	}
}
