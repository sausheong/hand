package sessionio

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestForkStagingCleanupProtectsActiveAndSymlink(t *testing.T) {
	m, err := NewManager(t.TempDir(), t.TempDir(), "hand")
	if err != nil {
		t.Fatal(err)
	}
	active, release, err := m.newForkStaging()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	stale, err := os.MkdirTemp(m.root, ".fork-")
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	sentinel := filepath.Join(outside, "keep")
	if err := os.WriteFile(sentinel, []byte("preserved"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(m.root, ".fork-link")); err != nil {
		t.Fatal(err)
	}
	count, err := m.CleanupForkStaging(context.Background())
	if err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale staging survived", err)
	}
	if _, err := os.Stat(active); err != nil {
		t.Fatal("active staging removed", err)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatal("followed staging symlink", err)
	}
}

func TestForkStagingCleanupBoundedAndCancelled(t *testing.T) {
	m, err := NewManager(t.TempDir(), t.TempDir(), "hand")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxForkStagingCleanup+1; i++ {
		if _, err := os.MkdirTemp(m.root, ".fork-"); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.CleanupForkStaging(ctx); err != context.Canceled {
		t.Fatal(err)
	}
	count, err := m.CleanupForkStaging(context.Background())
	if err != nil || count != maxForkStagingCleanup {
		t.Fatal(count, err)
	}
	count, err = m.CleanupForkStaging(context.Background())
	if err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

func TestForkStagingCleanupBeforeFirstSession(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	m, err := NewManager(root, t.TempDir(), "hand")
	if err != nil {
		t.Fatal(err)
	}
	if count, err := m.CleanupForkStaging(context.Background()); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	selected, err := m.Open(context.Background(), "", false)
	if err != nil {
		t.Fatal("first session failed", err)
	}
	selected.Session.Close()
}
