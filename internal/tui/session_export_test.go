package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sausheong/harness/session"
)

func TestSessionExportRetainsGraphAndSelection(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	sess := m.controller.Rt.Session
	leaf := sess.LeafID()
	sess.Append(session.UserMessageEntry("other branch"))
	if err := sess.Branch(leaf); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "hand"), 0700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "hand", "export with spaces.jsonl")
	before := m.identity
	m.Update(m.handleCommand("/export " + destination)())
	if m.identity != before {
		t.Fatal("export changed identity")
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("export permissions", info.Mode())
	}
	store := session.NewStore(dir)
	restored, err := store.LoadExclusive("hand", "export with spaces")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if restored.ID != sess.ID || restored.LeafID() != leaf || len(restored.Entries()) != len(sess.Entries()) {
		t.Fatal("export lost identity/graph/selection")
	}
	if err := m.controller.ExportSession(context.Background(), destination); err == nil {
		t.Fatal("export overwrote existing file")
	}
	after, err := os.ReadFile(destination)
	if err != nil || string(after) != string(data) {
		t.Fatal("existing destination changed", err)
	}
}
