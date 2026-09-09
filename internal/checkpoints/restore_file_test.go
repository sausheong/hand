package checkpoints

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreFileTransactionsPreserveStartingState(t *testing.T) {
	for _, operation := range []string{"replace", "create", "remove"} {
		t.Run(operation, func(t *testing.T) {
			dir := t.TempDir()
			if operation != "remove" {
				put(t, dir, "file", "user starting bytes", 0755)
			}
			before := capture(t, dir)
			if operation == "create" {
				if err := os.Remove(filepath.Join(dir, "file")); err != nil {
					t.Fatal(err)
				}
			} else {
				put(t, dir, "file", "agent bytes", 0600)
				if err := os.Chmod(filepath.Join(dir, "file"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			after := capture(t, dir)
			plan, err := PlanRestore(before, after, after, []string{"file"})
			if err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			var phases []string
			result, err := restoreFile(context.Background(), root, plan.Actions()[0], before, func(e RestoreEvent) error {
				phases = append(phases, e.Phase)
				if e.Phase == "prepared" {
					if operation != "create" {
						b, _ := os.ReadFile(filepath.Join(dir, "file"))
						if string(b) != "agent bytes" {
							t.Error("mutation before intent")
						}
					}
				}
				return nil
			})
			if err != nil || !result.Applied || len(phases) != 2 || phases[0] != "prepared" || phases[1] != "applied" {
				t.Fatal(result, err, phases)
			}
			if operation == "remove" {
				if _, err = os.Stat(filepath.Join(dir, "file")); !os.IsNotExist(err) {
					t.Fatal("created file not removed")
				}
			} else {
				b, _ := os.ReadFile(filepath.Join(dir, "file"))
				info, _ := os.Stat(filepath.Join(dir, "file"))
				if string(b) != "user starting bytes" || info.Mode().Perm() != 0755 {
					t.Fatal("starting content/mode not restored")
				}
			}
			if operation != "create" {
				b, err := os.ReadFile(filepath.Join(dir, result.RecoveryPath))
				if err != nil || string(b) != "agent bytes" {
					t.Fatal("displaced bytes not retained", err)
				}
			}
		})
	}
}
func TestRestoreFileIntentFailureAndLaterEdit(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "file", "before", 0600)
	before := capture(t, dir)
	put(t, dir, "file", "after!", 0600)
	after := capture(t, dir)
	plan, err := PlanRestore(before, after, after, []string{"file"})
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	injected := errors.New("intent write failed")
	r, err := restoreFile(context.Background(), root, plan.Actions()[0], before, func(RestoreEvent) error { return injected })
	if !errors.Is(err, injected) || r.Applied {
		t.Fatal(r, err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "file"))
	if string(b) != "after!" {
		t.Fatal("failed intent changed file")
	}
	r, err = restoreFile(context.Background(), root, plan.Actions()[0], before, func(e RestoreEvent) error {
		if e.Phase == "prepared" {
			return os.WriteFile(filepath.Join(dir, "file"), []byte("user later edit"), 0600)
		}
		return nil
	})
	if err == nil || r.Applied {
		t.Fatal("later edit not rejected", r, err)
	}
	b, _ = os.ReadFile(filepath.Join(dir, "file"))
	if string(b) != "user later edit" {
		t.Fatal("later edit lost")
	}
}
