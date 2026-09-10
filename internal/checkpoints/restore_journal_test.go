package checkpoints

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestRestoreJournalTransactionSurvivesReopen(t *testing.T) {
	work := t.TempDir()
	put(t, work, "file", "before", 0600)
	before := capture(t, work)
	put(t, work, "file", "after", 0600)
	after := capture(t, work)
	plan, err := PlanRestore(before, after, after, []string{"file"})
	if err != nil {
		t.Fatal(err)
	}
	store, dir := newStore(t, work, DefaultStoreLimits())
	if err = store.Save(context.Background(), before); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(work)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	result, err := restoreFile(context.Background(), root, plan.Actions()[0], before, store.RecordRestore)
	if err != nil || !result.Applied {
		t.Fatal(result, err)
	}
	store.Close()
	reopened, err := OpenStore(dir, work, DefaultStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	events, err := reopened.RestoreEvents(context.Background())
	if err != nil || len(events) != 2 || events[0].Phase != "prepared" || events[1].Phase != "applied" || events[0].RecoveryName != events[1].RecoveryName {
		t.Fatal(events, err)
	}
	b, _ := os.ReadFile(filepath.Join(work, result.RecoveryPath))
	if string(b) != "after" {
		t.Fatal("recovery bytes lost")
	}
	if err = reopened.RecordRestore(events[1]); err == nil {
		t.Fatal("duplicate application accepted")
	}
	ids, err := reopened.List()
	if err != nil || len(ids) != 1 {
		t.Fatal("journal corrupted snapshot inventory", ids, err)
	}
}
func TestRestoreJournalTruncatedRecordFailsReopen(t *testing.T) {
	work := t.TempDir()
	store, dir := newStore(t, work, DefaultStoreLimits())
	store.Close()
	if err := os.WriteFile(filepath.Join(dir, restoreJournalName), []byte(`{"version":1`), 0600); err != nil {
		t.Fatal(err)
	}
	if reopened, err := OpenStore(dir, work, DefaultStoreLimits()); err == nil {
		reopened.Close()
		t.Fatal("truncated journal accepted")
	}
}
func TestRestoreJournalSyncFailurePreventsMutation(t *testing.T) {
	work := t.TempDir()
	put(t, work, "file", "before", 0600)
	before := capture(t, work)
	put(t, work, "file", "after", 0600)
	after := capture(t, work)
	plan, err := PlanRestore(before, after, after, []string{"file"})
	if err != nil {
		t.Fatal(err)
	}
	store, _ := newStore(t, work, DefaultStoreLimits())
	store.syncFile = func(*os.File) error { return syscall.EIO }
	root, err := os.OpenRoot(work)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	result, err := restoreFile(context.Background(), root, plan.Actions()[0], before, store.RecordRestore)
	if !errors.Is(err, syscall.EIO) || result.Applied {
		t.Fatal(result, err)
	}
	b, _ := os.ReadFile(filepath.Join(work, "file"))
	if string(b) != "after" {
		t.Fatal("mutation before durable intent")
	}
	if _, err = store.List(); err == nil {
		t.Fatal("uncertain journal did not poison store")
	}
}

func TestRestoreAppliedJournalFailurePreservesRecoveryAndPoisonsStore(t *testing.T) {
	for _, boundary := range []string{"file", "directory"} {
		t.Run(boundary, func(t *testing.T) {
			work := t.TempDir()
			put(t, work, "file", "before", 0600)
			before := capture(t, work)
			put(t, work, "file", "after", 0600)
			after := capture(t, work)
			plan, err := PlanRestore(before, after, after, []string{"file"})
			if err != nil {
				t.Fatal(err)
			}
			store, dir := newStore(t, work, DefaultStoreLimits())
			for _, snapshot := range []*Snapshot{before, after} {
				if err = store.Save(context.Background(), snapshot); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			if boundary == "file" {
				store.syncFile = func(f *os.File) error {
					calls++
					if calls == 2 {
						return syscall.EIO
					}
					return f.Sync()
				}
			} else {
				original := store.syncDir
				store.syncDir = func() error {
					calls++
					if calls == 2 {
						return syscall.EIO
					}
					return original()
				}
			}
			results, err := store.ApplyRestore(context.Background(), plan, DefaultLimits())
			if !errors.Is(err, syscall.EIO) || len(results) != 1 || !results[0].Applied || results[0].RecoveryPath == "" {
				t.Fatalf("missing uncertain applied result: %+v %v", results, err)
			}
			for name, want := range map[string]string{"file": "before", results[0].RecoveryPath: "after"} {
				data, err := os.ReadFile(filepath.Join(work, name))
				if err != nil || string(data) != want {
					t.Fatalf("recovery bytes lost at %s: %q %v", name, data, err)
				}
			}
			if _, err = store.RestoreEvents(context.Background()); err == nil {
				t.Fatal("uncertain durability allowed store reuse")
			}
			if err = store.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenStore(dir, work, DefaultStoreLimits())
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			events, err := reopened.RestoreEvents(context.Background())
			if err != nil || len(events) != 2 || events[0].Phase != "prepared" || events[1].Phase != "applied" {
				t.Fatalf("written journal could not replay: %+v %v", events, err)
			}
			if _, err = reopened.ApplyRestore(context.Background(), plan, DefaultLimits()); err == nil {
				t.Fatal("obsolete restore plan replayed")
			}
		})
	}
}
