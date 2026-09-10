//go:build linux || darwin

package checkpoints

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreAtomicExchangeRetainsDisplacedFile(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "current", "after image", 0644)
	put(t, dir, "staged", "before image", 0755)
	parent, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if err = exchangeFiles(parent, "staged", "current"); err != nil {
		t.Fatal(err)
	}
	current, _ := os.ReadFile(filepath.Join(dir, "current"))
	displaced, _ := os.ReadFile(filepath.Join(dir, "staged"))
	info, _ := os.Stat(filepath.Join(dir, "current"))
	if string(current) != "before image" || string(displaced) != "after image" || info.Mode().Perm() != 0755 {
		t.Fatal("exchange lost bytes or mode")
	}
	if err = exchangeFiles(parent, "staged", "current"); err != nil {
		t.Fatal(err)
	}
	current, _ = os.ReadFile(filepath.Join(dir, "current"))
	if string(current) != "after image" {
		t.Fatal("reverse exchange failed")
	}
}
func TestRestoreExclusiveMoveCannotClobber(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "source", "saved", 0600)
	put(t, dir, "target", "later user file", 0600)
	parent, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if err = moveExclusive(parent, "source", "target"); err == nil {
		t.Fatal("existing target replaced")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "target"))
	if string(b) != "later user file" {
		t.Fatal("target clobbered")
	}
	if err = moveExclusive(parent, "source", "new"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(dir, "new"))
	if string(b) != "saved" {
		t.Fatal("move lost source")
	}
}
func TestRestoreExchangeDoesNotFollowSymlink(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	put(t, outside, "sentinel", "outside", 0600)
	put(t, dir, "staged", "restore", 0600)
	if err := os.Symlink(filepath.Join(outside, "sentinel"), filepath.Join(dir, "target")); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if err = exchangeFiles(parent, "staged", "target"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(outside, "sentinel"))
	if string(b) != "outside" {
		t.Fatal("followed link")
	}
	info, err := os.Lstat(filepath.Join(dir, "staged"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("displaced link not preserved")
	}
}
func TestRestoreRenameRejectsTraversalAndMissingTarget(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "staged", "saved", 0600)
	parent, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	for _, name := range []string{"../escape", "/absolute", ".", "", "a/b"} {
		if err = exchangeFiles(parent, "staged", name); err == nil {
			t.Fatal("invalid leaf accepted", name)
		}
	}
	if err = exchangeFiles(parent, "staged", "missing"); err == nil {
		t.Fatal("missing exchange target accepted")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "staged"))
	if string(b) != "saved" {
		t.Fatal("failed exchange lost stage")
	}
}

func TestRestoreRenameInvalidInputsPreserveBothFiles(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "source", "saved source", 0600)
	put(t, dir, "target", "user target", 0644)
	parent, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	regular, err := os.Open(filepath.Join(dir, "source"))
	if err != nil {
		t.Fatal(err)
	}
	defer regular.Close()
	closed, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	for label, rename := range map[string]func(*os.File, string, string) error{"exchange": exchangeFiles, "exclusive": moveExclusive} {
		t.Run(label, func(t *testing.T) {
			for _, name := range []string{"../escape", "/absolute", ".", "..", "", "a/b", "nul\x00leaf"} {
				if err := rename(parent, "source", name); err == nil {
					t.Fatalf("invalid target %q accepted", name)
				}
				if err := rename(parent, name, "target"); err == nil {
					t.Fatalf("invalid source %q accepted", name)
				}
			}
			if err := rename(parent, "source", "source"); err == nil {
				t.Fatal("identical leaves accepted")
			}
			for _, handle := range []*os.File{nil, regular, closed} {
				if err := rename(handle, "source", "target"); err == nil {
					t.Fatal("invalid directory handle accepted")
				}
			}
			for name, want := range map[string]string{"source": "saved source", "target": "user target"} {
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil || string(data) != want {
					t.Fatalf("rejected rename changed %s: %q %v", name, data, err)
				}
			}
		})
	}
}
