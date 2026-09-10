package sessionio

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sausheong/harness/session"
)

func managerFixture(t *testing.T) *Manager {
	t.Helper()
	m, err := NewManager(t.TempDir(), t.TempDir(), "hand")
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func TestManagerNewPreservesLegacyAndResumesLastActive(t *testing.T) {
	m := managerFixture(t)
	key := m.legacyKeys[0]
	path := filepath.Join(m.root, m.agent, key+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"id":"old","type":"message","role":"user","timestamp":123,"data":{"text":"previous work"}}` + "\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	fresh, err := m.Open(context.Background(), "", true)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Session.Close()
	if len(fresh.Session.Entries()) != 0 || len(fresh.Backups) != 1 {
		t.Fatal("fresh session or import backup missing", fresh.Backups)
	}
	backup, err := os.ReadFile(fresh.Backups[0])
	if err != nil || !bytes.Equal(backup, original) {
		t.Fatal("only copy of history lost", err)
	}
	snapshot, err := m.catalogue.Snapshot()
	if err != nil || len(snapshot.Sessions) != 2 || snapshot.LastActiveID != fresh.Session.ID {
		t.Fatal("new session replaced legacy catalogue", snapshot, err)
	}
	oldID := snapshot.Sessions[0].ID
	fresh.Session.Append(session.UserMessageEntry("new work"))
	if err := fresh.Session.Flush(); err != nil {
		t.Fatal(err)
	}
	fresh.Session.Close()
	resumed, err := m.Open(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Session.ID != fresh.Session.ID || len(resumed.Session.History()) != 1 {
		t.Fatal("auto-resume lost new session")
	}
	resumed.Session.Close()
	old, err := m.Open(context.Background(), oldID, false)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Session.Close()
	if len(old.Session.History()) != 1 || old.Session.History()[0].ID != "old" {
		t.Fatal("legacy history no longer resumable")
	}
	again, err := m.Reconcile(context.Background())
	if err != nil || len(again) != 0 {
		t.Fatal("repeat import duplicated backup", err)
	}
	snapshot, _ = m.catalogue.Snapshot()
	if len(snapshot.Sessions) != 2 {
		t.Fatal("repeat import duplicated sessions")
	}
}

func TestManagerReconcilesOrphanAndExcludesForeignWorkspace(t *testing.T) {
	m := managerFixture(t)
	if err := m.store.Create(m.agent, m.prefix+"orphan"); err != nil {
		t.Fatal(err)
	}
	if err := m.store.Create(m.agent, "workspace_other_foreign"); err != nil {
		t.Fatal(err)
	}
	selected, err := m.Open(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer selected.Session.Close()
	if selected.Record.StoreKey != m.prefix+"orphan" {
		t.Fatal("orphan not recovered", selected.Record)
	}
	snapshot, _ := m.catalogue.Snapshot()
	if len(snapshot.Sessions) != 1 {
		t.Fatal("foreign session imported")
	}
}

func TestManagerWriterOwnershipAndIndependentNewSession(t *testing.T) {
	m := managerFixture(t)
	first, err := m.Open(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Session.Close()
	other, err := NewManager(m.root, m.catalogue.workspace, m.agent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Open(context.Background(), first.Session.ID, false); !errors.Is(err, session.ErrSessionBusy) {
		t.Fatal("second writer admitted", err)
	}
	second, err := other.Open(context.Background(), "", true)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Session.Close()
	if second.Session.ID == first.Session.ID {
		t.Fatal("new session reused identity")
	}
	first.Session.Append(session.UserMessageEntry("still writable"))
	if err := first.Session.Flush(); err != nil {
		t.Fatal(err)
	}
}

func TestManagerDetectsMissingAndReplacedBackend(t *testing.T) {
	for _, missing := range []bool{false, true} {
		m := managerFixture(t)
		selected, err := m.Open(context.Background(), "", false)
		if err != nil {
			t.Fatal(err)
		}
		selected.Session.Close()
		path, err := m.SessionPath(selected.Record)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if !missing {
			if err := m.store.Create(m.agent, selected.Record.StoreKey); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := m.Open(context.Background(), selected.Record.ID, false); err == nil {
			t.Fatal("missing/replaced backend silently accepted")
		}
	}
}

func TestManagerRecoversTailAndRejectsInteriorCorruption(t *testing.T) {
	for _, tail := range []bool{false, true} {
		m := managerFixture(t)
		path := filepath.Join(m.root, m.agent, m.legacyKeys[0]+".jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		original := `{"id":"old","type":"message","data":{"text":"keep"}}` + "\n" + `{"id":`
		if !tail {
			original += "\n" + `{"id":"later"}`
		}
		if err := os.WriteFile(path, []byte(original), 0600); err != nil {
			t.Fatal(err)
		}
		selected, err := m.Open(context.Background(), "", false)
		if tail {
			if err != nil {
				t.Fatal(err)
			}
			selected.Session.Close()
			if len(selected.Backups) < 1 || len(selected.Session.Entries()) != 1 {
				t.Fatal("tail recovery missing")
			}
		} else {
			if err == nil {
				selected.Session.Close()
				t.Fatal("interior corruption dropped")
			}
			raw, _ := os.ReadFile(path)
			if string(raw) != original {
				t.Fatal("interior corruption modified")
			}
		}
	}
}

func TestManagerRejectsCancelledAndConflictingSelection(t *testing.T) {
	m := managerFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Create(ctx, "name"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := m.Open(context.Background(), "id", true); err == nil {
		t.Fatal("conflicting flags accepted")
	}
	snapshot, err := m.catalogue.Snapshot()
	if err != nil || len(snapshot.Sessions) != 0 {
		t.Fatal("rejected request created catalogue", err)
	}
}
