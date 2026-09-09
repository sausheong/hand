package packages

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

func TestPackageRecoveryRemovesOnlyPrivateReservedRegularFiles(t *testing.T) {
	dir := privateStageParent(t)
	stale := "lock-" + rand.Text() + ".tmp"
	if err := os.WriteFile(filepath.Join(dir, stale), []byte(`{"partial":`), 0600); err != nil {
		t.Fatal(err)
	}
	unrelated := "lock-not-a-generated-name.tmp"
	if err := os.WriteFile(filepath.Join(dir, unrelated), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("keep target"), 0600); err != nil {
		t.Fatal(err)
	}
	link := "lock-" + rand.Text() + ".tmp"
	if err := os.Symlink(target, filepath.Join(dir, link)); err != nil {
		t.Fatal(err)
	}
	folder := "lock-" + rand.Text() + ".tmp"
	if err := os.Mkdir(filepath.Join(dir, folder), 0700); err != nil {
		t.Fatal(err)
	}
	public := "lock-" + rand.Text() + ".tmp"
	if err := os.WriteFile(filepath.Join(dir, public), []byte("keep public"), 0644); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(dir, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if got := store.Recovery(); got.RemovedTemporaryLocks != 1 || got.PreservedUnsafeEntries != 3 {
		t.Fatal("recovery report", got)
	}
	if _, err = os.Stat(filepath.Join(dir, stale)); !os.IsNotExist(err) {
		t.Fatal("stale file survived")
	}
	for _, name := range []string{unrelated, link, folder, public} {
		if _, err = os.Lstat(filepath.Join(dir, name)); err != nil {
			t.Fatal("unowned entry removed", name, err)
		}
	}
	if raw, err := os.ReadFile(target); err != nil || string(raw) != "keep target" {
		t.Fatal("symlink target modified")
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(dir, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.Recovery().RemovedTemporaryLocks != 0 {
		t.Fatal("recovery was not idempotent")
	}
}
func TestPackageRecoveryDoesNotModifyCorruptStore(t *testing.T) {
	dir := privateStageParent(t)
	stale := "lock-" + rand.Text() + ".tmp"
	if err := os.WriteFile(filepath.Join(dir, stale), []byte("uncommitted"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lock.json"), []byte(`{"invalid":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if store, err := OpenStore(dir, "1.0.0"); err == nil || store != nil {
		t.Fatal("corrupt store admitted")
	}
	if raw, err := os.ReadFile(filepath.Join(dir, stale)); err != nil || string(raw) != "uncommitted" {
		t.Fatal("corrupt-store evidence changed")
	}
}
