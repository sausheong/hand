package checkpoints

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func put(t *testing.T, root, name, text string, mode os.FileMode) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), mode); err != nil {
		t.Fatal(err)
	}
}
func capture(t *testing.T, root string) *Snapshot {
	t.Helper()
	s, err := Capture(context.Background(), root, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestDirtyUntrackedNonGitSnapshot(t *testing.T) {
	root := t.TempDir()
	put(t, root, "existing", "user dirty content", 0600)
	put(t, root, "untracked", "new file", 0644)
	first := capture(t, root)
	if len(first.Records()) != 2 {
		t.Fatal(first.Records())
	}
	if first.Digest() != capture(t, root).Digest() {
		t.Fatal("unstable digest")
	}
	content, _ := os.ReadFile(filepath.Join(root, "existing"))
	if string(content) != "user dirty content" {
		t.Fatal("capture changed user data")
	}
	put(t, root, "existing", "agent edit", 0600)
	if err := os.Remove(filepath.Join(root, "untracked")); err != nil {
		t.Fatal(err)
	}
	put(t, root, "added", "added", 0644)
	next := capture(t, root)
	changes := Changes(first, next)
	if len(changes) != 3 || changes[0].Path != "added" || changes[0].Before != nil || changes[2].After != nil {
		t.Fatal(changes)
	}
	if first.Digest() == next.Digest() {
		t.Fatal("edit did not invalidate snapshot")
	}
}
func TestModeAndImmutableContent(t *testing.T) {
	root := t.TempDir()
	put(t, root, "script", "echo hi", 0644)
	first := capture(t, root)
	r := first.Records()
	hash := r[0].Hash
	r[0].Hash = "mutated"
	b, ok := first.Content(hash)
	if !ok {
		t.Fatal("missing content")
	}
	b[0] = 'X'
	again, _ := first.Content(hash)
	if string(again) != "echo hi" || first.Records()[0].Hash != hash {
		t.Fatal("mutable snapshot")
	}
	if err := os.Chmod(filepath.Join(root, "script"), 0755); err != nil {
		t.Fatal(err)
	}
	next := capture(t, root)
	c := Changes(first, next)
	if len(c) != 1 || c[0].Before.Mode != 0644 || c[0].After.Mode != 0755 || first.Digest() == next.Digest() {
		t.Fatal(c)
	}
}
func TestSensitivePathsAndSymlinkNotRead(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	put(t, external, "secret", "outside", 0600)
	put(t, root, ".env.local", "secret", 0600)
	put(t, root, ".git/config", "gitsecret", 0600)
	put(t, root, "safe", "public", 0600)
	if err := os.Symlink(filepath.Join(external, "secret"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	s := capture(t, root)
	if len(s.Records()) != 1 || len(s.Omissions()) != 3 {
		t.Fatal(s.Records(), s.Omissions())
	}
	for _, b := range s.content {
		if string(b) != "public" {
			t.Fatal("captured excluded bytes")
		}
	}
	l := DefaultLimits()
	l.Exclude = []string{"safe"}
	other, err := Capture(context.Background(), root, l)
	if err != nil {
		t.Fatal(err)
	}
	if len(other.Records()) != 0 || other.Digest() == s.Digest() {
		t.Fatal("scope absent from digest")
	}
}
func TestLimitsFailWholeCapture(t *testing.T) {
	for _, kind := range []string{"file", "total", "entries"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			put(t, root, "one", "12345", 0600)
			put(t, root, "two", "67890", 0600)
			l := DefaultLimits()
			switch kind {
			case "file":
				l.MaxFileBytes = 4
			case "total":
				l.MaxTotalBytes = 9
			case "entries":
				l.MaxFiles = 1
			}
			s, err := Capture(context.Background(), root, l)
			if err == nil || s != nil || !strings.Contains(err.Error(), "limit") {
				t.Fatal(s, err)
			}
		})
	}
}
func TestCancelledAndInvalidScope(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if s, err := Capture(ctx, root, DefaultLimits()); err == nil || s != nil {
		t.Fatal(s, err)
	}
	l := DefaultLimits()
	l.Exclude = []string{"../escape"}
	if _, err := Capture(context.Background(), root, l); err == nil {
		t.Fatal("invalid exclusion accepted")
	}
	l = DefaultLimits()
	l.MaxFiles = 0
	if _, err := Capture(context.Background(), root, l); err == nil {
		t.Fatal("invalid limit accepted")
	}
}

func TestCheckpointOmitsRestoreRecoveryContent(t *testing.T) {
	work := t.TempDir()
	put(t, work, "project.txt", "project", 0600)
	before := capture(t, work)
	recovery := ".hand-restore-0123456789abcdef0123456789abcdef"
	put(t, work, recovery, "retained displaced bytes", 0600)
	put(t, work, "nested/"+recovery, "retained nested bytes", 0600)
	after := capture(t, work)
	if len(after.Records()) != 1 || after.Records()[0].Path != "project.txt" {
		t.Fatal("recovery content captured as project data", after.Records())
	}
	if len(after.Omissions()) != 2 {
		t.Fatal("recovery omissions not disclosed", after.Omissions())
	}
	if len(Changes(before, after)) != 0 {
		t.Fatal("recovery file appeared in project changes")
	}
	plan, err := PlanRestore(before, after, after, []string{recovery})
	if err != nil || !plan.HasConflicts() {
		t.Fatal("recovery path restore accepted", plan, err)
	}
	data, err := os.ReadFile(filepath.Join(work, recovery))
	if err != nil || string(data) != "retained displaced bytes" {
		t.Fatal("capture altered recovery data", string(data), err)
	}
	put(t, work, ".hand-restore-not-a-generated-id", "ordinary file", 0600)
	final := capture(t, work)
	if len(final.Records()) != 2 {
		t.Fatal("non-reserved filename omitted", final.Records())
	}
}
