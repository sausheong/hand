package checkpoints

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestorePreviewPreservesDirtyAndLaterEdits(t *testing.T) {
	root := t.TempDir()
	put(t, root, "dirty", "user starting edit", 0600)
	put(t, root, "other", "original", 0600)
	before := capture(t, root)
	put(t, root, "dirty", "agent edit", 0600)
	put(t, root, "other", "agent other edit", 0600)
	after := capture(t, root)
	put(t, root, "other", "later user edit", 0600)
	current := capture(t, root)
	plan, err := PlanRestore(before, after, current, []string{"dirty", "other"})
	if err != nil {
		t.Fatal(err)
	}
	oldID, agentID, reviewedID := plan.Snapshots()
	if oldID != before.Digest() || agentID != after.Digest() || reviewedID != current.Digest() {
		t.Fatal("preview did not bind the original, agent and later user states", oldID, agentID, reviewedID)
	}
	actions := plan.Actions()
	if len(actions) != 2 || actions[0].Conflict != "" || actions[1].Conflict == "" || !plan.HasConflicts() {
		t.Fatal(actions)
	}
	b, ok := before.Content(actions[0].Restore.Hash)
	if !ok || string(b) != "user starting edit" {
		t.Fatal("wrong restore target")
	}
	actual, _ := os.ReadFile(filepath.Join(root, "dirty"))
	if string(actual) != "agent edit" {
		t.Fatal("preview wrote files")
	}
	actions[0].Restore.Hash = "modified"
	if plan.Actions()[0].Restore.Hash == "modified" {
		t.Fatal("mutable preview")
	}
}
func TestRestorePreviewCreatesRemovesAndModes(t *testing.T) {
	root := t.TempDir()
	put(t, root, "deleted", "old untracked", 0755)
	put(t, root, "mode", "script", 0644)
	before := capture(t, root)
	if err := os.Remove(filepath.Join(root, "deleted")); err != nil {
		t.Fatal(err)
	}
	put(t, root, "created", "agent file", 0600)
	if err := os.Chmod(filepath.Join(root, "mode"), 0755); err != nil {
		t.Fatal(err)
	}
	after := capture(t, root)
	plan, err := PlanRestore(before, after, after, []string{"mode", "created", "deleted"})
	if err != nil {
		t.Fatal(err)
	}
	a := plan.Actions()
	if plan.HasConflicts() || a[0].Operation != "remove" || a[1].Operation != "create" || a[1].Restore.Mode != 0755 || a[2].Restore.Mode != 0644 {
		t.Fatal(a)
	}
	if err := os.Chmod(filepath.Join(root, "mode"), 0700); err != nil {
		t.Fatal(err)
	}
	p, err := PlanRestore(before, after, capture(t, root), []string{"mode"})
	if err != nil || !p.HasConflicts() {
		t.Fatal(p, err)
	}
}
func TestRestorePreviewSymlinkAndExclusionsConflict(t *testing.T) {
	root := t.TempDir()
	put(t, root, "file", "before", 0600)
	before := capture(t, root)
	put(t, root, "file", "after", 0600)
	after := capture(t, root)
	if err := os.Remove(filepath.Join(root, "file")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(root, "file")); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanRestore(before, after, capture(t, root), []string{"file", ".env"})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range plan.Actions() {
		if a.Conflict == "" {
			t.Fatal("omitted path mistaken for absent file", a)
		}
	}
}
func TestRestorePreviewSelectionValidation(t *testing.T) {
	root := t.TempDir()
	s := capture(t, root)
	for _, selection := range [][]string{nil, {"../escape"}, {"one", "one"}, {"."}} {
		if _, err := PlanRestore(s, s, s, selection); err == nil {
			t.Fatal("invalid selection accepted", selection)
		}
	}
	p, err := PlanRestore(s, s, s, []string{"absent"})
	if err != nil || !p.HasConflicts() {
		t.Fatal(p, err)
	}
}
