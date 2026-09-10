package checkpoints

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the persisted trust boundary, not only the event validator: a bad
// journal must fail reopen without changing either the journal or user files.
func TestRestoreJournalCorruptionPreservesEvidenceAndReleasesLock(t *testing.T) {
	cases := []struct {
		name string
		edit func(*restoreJournalRecord)
	}{
		{"foreign-workspace", func(r *restoreJournalRecord) { r.Workspace += "-other" }},
		{"future-version", func(r *restoreJournalRecord) { r.Version++ }},
		{"traversal", func(r *restoreJournalRecord) { r.Event.Path = "../sentinel" }},
		{"invalid-recovery-id", func(r *restoreJournalRecord) { r.Event.RecoveryName = ".hand-restore-xyz" }},
		{"unknown-phase", func(r *restoreJournalRecord) { r.Event.Phase = "committed" }},
		{"resolution-without-intent", func(r *restoreJournalRecord) { r.Event.Phase = "applied" }},
		{"record-target-substitution", func(r *restoreJournalRecord) { r.Event.Expected.Path = "other" }},
		{"invalid-content-hash", func(r *restoreJournalRecord) { r.Event.Expected.Hash = "unknown" }},
		{"negative-size", func(r *restoreJournalRecord) { r.Event.Expected.Size = -1 }},
		{"privileged-mode", func(r *restoreJournalRecord) { r.Event.Restored.Mode = 04755 }},
		{"create-with-existing-image", func(r *restoreJournalRecord) { r.Event.Operation = "create" }},
		{"remove-with-restore-image", func(r *restoreJournalRecord) { r.Event.Operation = "remove" }},
		{"replace-without-image", func(r *restoreJournalRecord) { r.Event.Restored = nil }},
		{"unknown-operation", func(r *restoreJournalRecord) { r.Event.Operation = "execute" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work := t.TempDir()
			put(t, work, "file", "user original", 0640)
			before := capture(t, work)
			put(t, work, "file", "agent change", 0600)
			after := capture(t, work)
			store, dir := newStore(t, work, DefaultStoreLimits())
			old, current := before.Records()[0], after.Records()[0]
			event := RestoreEvent{Path: "file", Operation: "replace", Phase: "prepared",
				RecoveryName: ".hand-restore-" + strings.Repeat("a", 32), Expected: &current, Restored: &old}
			if err := store.RecordRestore(event); err != nil {
				t.Fatal(err)
			}
			journal := filepath.Join(dir, restoreJournalName)
			valid, err := os.ReadFile(journal)
			if err != nil {
				t.Fatal(err)
			}
			if err = store.Close(); err != nil {
				t.Fatal(err)
			}
			var record restoreJournalRecord
			if err = json.Unmarshal(valid, &record); err != nil {
				t.Fatal(err)
			}
			tc.edit(&record)
			bad, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			bad = append(bad, '\n')
			if err = os.WriteFile(journal, bad, 0600); err != nil {
				t.Fatal(err)
			}
			if reopened, err := OpenStore(dir, work, DefaultStoreLimits()); err == nil {
				reopened.Close()
				t.Fatal("corrupt journal accepted")
			}
			got, err := os.ReadFile(journal)
			if err != nil || !bytes.Equal(got, bad) {
				t.Fatal("failed reopen rewrote forensic evidence", err)
			}
			if now := capture(t, work); now.Digest() != after.Digest() {
				t.Fatal("failed reopen mutated workspace")
			}
			// Repair only the injected corruption; a failed open must have
			// released ownership so the authentic journal can be reopened.
			if err = os.WriteFile(journal, valid, 0600); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenStore(dir, work, DefaultStoreLimits())
			if err != nil {
				t.Fatal("failed open leaked ownership", err)
			}
			defer reopened.Close()
			events, err := reopened.RestoreEvents(context.Background())
			if err != nil || len(events) != 1 || events[0].Phase != "prepared" {
				t.Fatal("authentic intent lost", events, err)
			}
		})
	}
}
