package checkpoints

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyRestoreReviewedSelection(t *testing.T) {
	for _, scenario := range []string{"apply", "stale", "tampered", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			work := t.TempDir()
			put(t, work, "changed", "dirty original", 0755)
			put(t, work, "deleted", "restore me", 0600)
			before := capture(t, work)
			put(t, work, "changed", "agent edit", 0600)
			put(t, work, "added", "agent created", 0600)
			if err := os.Remove(filepath.Join(work, "deleted")); err != nil {
				t.Fatal(err)
			}
			after := capture(t, work)
			store, _ := newStore(t, work, DefaultStoreLimits())
			for _, snap := range []*Snapshot{before, after} {
				if err := store.Save(context.Background(), snap); err != nil {
					t.Fatal(err)
				}
			}
			plan, err := PlanRestore(before, after, after, []string{"changed", "deleted", "added"})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch scenario {
			case "stale":
				put(t, work, "unrelated", "new user work", 0600)
			case "tampered":
				plan.actions[0].Expected.Hash = before.records[0].Hash
			case "cancelled":
				cancel()
			}
			result, err := store.ApplyRestore(ctx, plan, DefaultLimits())
			if scenario != "apply" {
				if err == nil || len(result) != 0 {
					t.Fatal(result, err)
				}
				b, _ := os.ReadFile(filepath.Join(work, "changed"))
				if string(b) != "agent edit" {
					t.Fatal("rejected restore changed workspace")
				}
				events, e := store.RestoreEvents(context.Background())
				if e != nil || len(events) != 0 {
					t.Fatal(events, e)
				}
				return
			}
			if err != nil || len(result) != 3 {
				t.Fatal(result, err)
			}
			root, e := os.OpenRoot(work)
			if e != nil {
				t.Fatal(e)
			}
			defer root.Close()
			for _, r := range before.Records() {
				if e = matchRestoreFile(ctx, root, r.Path, &r); e != nil {
					t.Fatal(e)
				}
			}
			if e = matchRestoreFile(ctx, root, "added", nil); e != nil {
				t.Fatal(e)
			}
			for _, r := range result {
				if !r.Applied {
					t.Fatal(r)
				}
			}
			states, e := store.InspectRestores(ctx)
			if e != nil || len(states) != 3 {
				t.Fatal(states, e)
			}
			for _, state := range states {
				if state.State != "applied" {
					t.Fatal(state)
				}
			}
		})
	}
}
