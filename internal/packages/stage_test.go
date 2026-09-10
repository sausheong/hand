package packages

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStagePinnedPackageIsIndependentAndOwned(t *testing.T) {
	source := writeFixture(t)
	parent := privateStageParent(t)
	_, pin, err := VerifyDirectory(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := StageDirectory(context.Background(), source, parent, pin)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	if snapshot.Digest() != pin {
		t.Fatal("pin changed")
	}
	staged := snapshot.Directory()
	if err = os.WriteFile(filepath.Join(source, "note.py"), []byte("changed source"), 0600); err != nil {
		t.Fatal(err)
	}
	_, got, err := VerifyDirectory(context.Background(), staged)
	if err != nil || got != pin {
		t.Fatal("snapshot aliased source", got, err)
	}
	for _, name := range []string{ManifestName, "note.py"} {
		info, err := os.Stat(filepath.Join(staged, name))
		if err != nil || info.Mode().Perm() != 0400 {
			t.Fatal("snapshot not read-only", name, err)
		}
	}
	if err = snapshot.Close(); err != nil {
		t.Fatal(err)
	}
	if err = snapshot.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("snapshot cleanup failed", entries, err)
	}
	raw, err := os.ReadFile(filepath.Join(source, "note.py"))
	if err != nil || string(raw) != "changed source" {
		t.Fatal("cleanup modified source", err)
	}
}
func TestStageRefusesWrongPinAndUnsafeParent(t *testing.T) {
	for _, mode := range []string{"wrong-pin", "missing-pin", "public-parent", "symlink-parent", "inside-source"} {
		t.Run(mode, func(t *testing.T) {
			source := writeFixture(t)
			parent := privateStageParent(t)
			_, pin, err := VerifyDirectory(context.Background(), source)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "wrong-pin":
				pin = strings.Repeat("0", 64)
			case "missing-pin":
				pin = ""
			case "public-parent":
				err = os.Chmod(parent, 0755)
			case "symlink-parent":
				link := filepath.Join(t.TempDir(), "parent")
				err = os.Symlink(parent, link)
				parent = link
			case "inside-source":
				parent = filepath.Join(source, "staging")
				err = os.Mkdir(parent, 0700)
			}
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := StageDirectory(context.Background(), source, parent, pin)
			if err == nil || snapshot != nil {
				t.Fatal("unsafe staging accepted", snapshot, err)
			}
			entries, err := os.ReadDir(parent)
			if err != nil || len(entries) != 0 {
				t.Fatal("refusal left output", entries, err)
			}
		})
	}
}

// Cancellation is injected when a partial file first exists, without relying
// on copying speed or sleeps. Done and Err come from the same cancelled context.
type cancelOnPartialFile struct {
	context.Context
	cancel context.CancelFunc
	parent string
}

func (c *cancelOnPartialFile) Err() error {
	entries, _ := os.ReadDir(c.parent)
	for _, entry := range entries {
		if _, err := os.Stat(filepath.Join(c.parent, entry.Name(), "note.py")); err == nil {
			c.cancel()
		}
	}
	return c.Context.Err()
}
func TestStageCancellationRemovesPartialCopy(t *testing.T) {
	source := writeFixture(t)
	parent := privateStageParent(t)
	_, pin, err := VerifyDirectory(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &cancelOnPartialFile{Context: base, cancel: cancel, parent: parent}
	snapshot, err := StageDirectory(ctx, source, parent, pin)
	if !errors.Is(err, context.Canceled) || snapshot != nil {
		t.Fatal("cancelled staging succeeded", snapshot, err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("partial copy survived", entries, err)
	}
	if _, _, err = VerifyDirectory(context.Background(), source); err != nil {
		t.Fatal("source damaged", err)
	}
}
func TestCopyRevalidatesSourceAfterReview(t *testing.T) {
	for _, mode := range []string{"content", "size", "executable", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			source := writeFixture(t)
			m, _, err := VerifyDirectory(context.Background(), source)
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(source, "note.py")
			switch mode {
			case "content":
				err = os.WriteFile(file, []byte("print('other')\n"), 0600)
			case "size":
				err = os.WriteFile(file, []byte("short"), 0600)
			case "executable":
				err = os.Chmod(file, 0700)
			case "symlink":
				if err = os.Remove(file); err == nil {
					err = os.Symlink("/etc/passwd", file)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			input, err := os.OpenRoot(source)
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			output, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			if err = copyPackageFile(context.Background(), input, output, m.Files[0]); err == nil {
				t.Fatal("source change escaped copy verification")
			}
		})
	}
}

func privateStageParent(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}
