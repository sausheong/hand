package checkpoints

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreRecoveryClassifiesActualTransactionStates(t *testing.T) {
	for _, phase := range []string{"prepared", "applied_unrecorded", "applied"} {
		t.Run(phase, func(t *testing.T) {
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
			root, err := os.OpenRoot(work)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, runErr := restoreFile(ctx, root, plan.Actions()[0], before, func(e RestoreEvent) error {
				if e.Phase == "applied" && phase == "applied_unrecorded" {
					return errors.New("interrupted before completion record")
				}
				if err := store.RecordRestore(e); err != nil {
					return err
				}
				if e.Phase == "prepared" && phase == "prepared" {
					cancel()
				}
				return nil
			})
			if phase == "applied" && runErr != nil {
				t.Fatal(runErr)
			}
			if phase != "applied" && runErr == nil {
				t.Fatal("interruption absent")
			}
			states, err := store.InspectRestores(context.Background())
			if err != nil || len(states) != 1 || states[0].State != phase {
				t.Fatal(states, err)
			}
			reviewed := states[0]
			if err = os.WriteFile(filepath.Join(work, "file"), []byte("later user edit"), 0600); err != nil {
				t.Fatal(err)
			}
			states, err = store.InspectRestores(context.Background())
			if err != nil || states[0].State != "conflict" {
				t.Fatal(states, err)
			}
			if err := store.ReconcileAppliedRestore(context.Background(), reviewed); err == nil {
				t.Fatal("conflicting recovery acknowledged")
			}
			if err := store.CancelPreparedRestore(context.Background(), reviewed); err == nil {
				t.Fatal("conflicting recovery cancelled")
			}
			b, _ := os.ReadFile(filepath.Join(work, "file"))
			if string(b) != "later user edit" {
				t.Fatal("inspection mutated user file")
			}
		})
	}
}
func TestRestoreParentRejectsWorkspaceSymlink(t *testing.T) {
	work := t.TempDir()
	if err := os.Mkdir(filepath.Join(work, "real"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(work, "alias")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(work)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if p, err := restoreParent(root, "alias"); err == nil {
		p.Close()
		t.Fatal("symlink parent accepted")
	}
}
