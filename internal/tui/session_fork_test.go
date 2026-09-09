package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sausheong/harness/session"
)

func TestSessionForkPreservesSourceAndCopiesSelectedBranch(t *testing.T) {
	m, manager, _ := sessionUIFixture(t)
	source := m.controller.Rt.Session
	root := source.LeafID()
	source.Append(session.UserMessageEntry("excluded branch"))
	if err := source.Branch(root); err != nil {
		t.Fatal(err)
	}
	before := m.identity
	m.Update(m.handleCommand("/fork")())
	fork := m.controller.Rt.Session
	if fork.ID == source.ID || m.identity.SessionID != fork.ID || m.identity.Generation <= before.Generation {
		t.Fatal("fork identity not committed")
	}
	if len(fork.History()) != 1 || fork.LeafID() != root {
		t.Fatal("fork did not copy selected history")
	}
	if strings.Contains(strings.Join(m.transcript, "\n"), "excluded branch") {
		t.Fatal("fork replay included unrelated branch")
	}
	fork.Append(session.UserMessageEntry("fork-only continuation"))
	if err := fork.Flush(); err != nil {
		t.Fatal(err)
	}
	original, err := manager.Open(context.Background(), source.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	defer original.Session.Close()
	if original.Session.LeafID() != root || len(original.Session.History()) != 1 || len(original.Session.Entries()) != 3 {
		t.Fatal("fork mutated source graph")
	}
	if err := fork.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := manager.Open(context.Background(), fork.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	m.controller.Rt.Session = reopened.Session
	if len(reopened.Session.History()) != 2 {
		t.Fatal("fork continuation lost on reopen")
	}
}
