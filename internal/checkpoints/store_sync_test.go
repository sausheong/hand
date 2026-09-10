package checkpoints

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestStoreFileSyncFailureDoesNotPublish(t *testing.T) {
	work := t.TempDir()
	put(t, work, "file", "accepted", 0600)
	before := capture(t, work)
	store, dir := newStore(t, work, DefaultStoreLimits())
	if err := store.Save(context.Background(), before); err != nil {
		t.Fatal(err)
	}
	put(t, work, "file", "new", 0600)
	after := capture(t, work)
	store.syncFile = func(*os.File) error { return syscall.ENOSPC }
	if err := store.Save(context.Background(), after); !errors.Is(err, syscall.ENOSPC) {
		t.Fatal("sync failure lost", err)
	}
	if _, err := store.Load(context.Background(), before.Digest()); err != nil {
		t.Fatal("accepted state lost", err)
	}
	if _, err := store.Load(context.Background(), after.Digest()); !os.IsNotExist(err) {
		t.Fatal("unsynced snapshot published", err)
	}
	pending, _ := filepath.Glob(filepath.Join(dir, ".pending-*"))
	if len(pending) != 0 {
		t.Fatal("failed sync leaked temporary content")
	}
	store.syncFile = (*os.File).Sync
	if err := store.Save(context.Background(), after); err != nil {
		t.Fatal("safe prepublication failure poisoned store", err)
	}
}

func TestStoreDirectorySyncFailurePoisonsUntilReopen(t *testing.T) {
	work := t.TempDir()
	put(t, work, "file", "snapshot", 0600)
	snap := capture(t, work)
	store, dir := newStore(t, work, DefaultStoreLimits())
	store.syncDir = func() error { return syscall.EIO }
	if err := store.Save(context.Background(), snap); !errors.Is(err, syscall.EIO) || !strings.Contains(err.Error(), "uncertain") {
		t.Fatal("uncertain durability reported as success", err)
	}
	if _, err := store.List(); err == nil {
		t.Fatal("uncertain store admitted list")
	}
	if _, err := store.Load(context.Background(), snap.Digest()); err == nil {
		t.Fatal("uncertain store admitted load")
	}
	if err := store.Delete(snap.Digest()); err == nil {
		t.Fatal("uncertain store admitted deletion")
	}
	store.Close()
	reopened, err := OpenStore(dir, work, DefaultStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err = reopened.Load(context.Background(), snap.Digest()); err != nil {
		t.Fatal("published bytes not recoverable", err)
	}
}
