package checkpoints

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newStore(t *testing.T, workspace string, limits StoreLimits) (*Store, string) {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "store")
	s, err := OpenStore(directory, workspace, limits)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, directory
}
func TestStoreRoundtripReopenAndDelete(t *testing.T) {
	ctx := context.Background()
	work := t.TempDir()
	put(t, work, "file", "dirty user bytes", 0755)
	snap := capture(t, work)
	s, dir := newStore(t, work, DefaultStoreLimits())
	if err := s.Save(ctx, snap); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(ctx, snap); err != nil {
		t.Fatal("idempotent", err)
	}
	ids, err := s.List()
	if err != nil || len(ids) != 1 {
		t.Fatal(ids, err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(dir, work, DefaultStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	loaded, err := s.Load(ctx, snap.Digest())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Digest() != snap.Digest() || len(Changes(snap, loaded)) != 0 {
		t.Fatal("roundtrip changed state")
	}
	b, _ := loaded.Content(loaded.Records()[0].Hash)
	if string(b) != "dirty user bytes" || loaded.Records()[0].Mode != 0755 {
		t.Fatal(loaded.Records())
	}
	if err = s.Delete(snap.Digest()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Load(ctx, snap.Digest()); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	current, _ := os.ReadFile(filepath.Join(work, "file"))
	if string(current) != "dirty user bytes" {
		t.Fatal("store mutated workspace")
	}
}
func TestStoreLockAndWorkspaceBoundary(t *testing.T) {
	work := t.TempDir()
	_, dir := newStore(t, work, DefaultStoreLimits())
	if s, err := OpenStore(dir, work, DefaultStoreLimits()); err == nil {
		s.Close()
		t.Fatal("second owner accepted")
	}
	inside := filepath.Join(work, "new", "store")
	if _, err := OpenStore(inside, work, DefaultStoreLimits()); err == nil {
		t.Fatal("workspace storage accepted")
	}
	if _, err := os.Stat(filepath.Join(work, "new")); !os.IsNotExist(err) {
		t.Fatal("created workspace state")
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(work, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(filepath.Join(alias, "store"), work, DefaultStoreLimits()); err == nil {
		t.Fatal("symlink workspace storage accepted")
	}
}
func TestStoreRetentionPreservesAcceptedSnapshots(t *testing.T) {
	ctx := context.Background()
	work := t.TempDir()
	put(t, work, "file", "first", 0600)
	one := capture(t, work)
	s, _ := newStore(t, work, StoreLimits{1, 1 << 20})
	if err := s.Save(ctx, one); err != nil {
		t.Fatal(err)
	}
	put(t, work, "file", "second", 0600)
	two := capture(t, work)
	if err := s.Save(ctx, two); err == nil {
		t.Fatal("retention limit ignored")
	}
	if _, err := s.Load(ctx, one.Digest()); err != nil {
		t.Fatal("old snapshot lost", err)
	}
	if err := s.Delete(one.Digest()); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(ctx, two); err != nil {
		t.Fatal(err)
	}
	small, _ := newStore(t, work, StoreLimits{10, 10})
	if err := small.Save(ctx, two); err == nil {
		t.Fatal("byte limit ignored")
	}
	ids, err := small.List()
	if err != nil || len(ids) != 0 {
		t.Fatal(ids, err)
	}
}
func TestStoreTamperAndSymlinkRejected(t *testing.T) {
	ctx := context.Background()
	work := t.TempDir()
	put(t, work, "file", "data", 0600)
	snap := capture(t, work)
	s, dir := newStore(t, work, DefaultStoreLimits())
	if err := s.Save(ctx, snap); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, snap.Digest()+".json")
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	b = []byte(strings.Replace(string(b), "ZGF0YQ==", "ZXZpbA==", 1))
	if err = os.WriteFile(file, b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Load(ctx, snap.Digest()); err == nil {
		t.Fatal("corrupt content accepted")
	}
	if err = s.Save(ctx, snap); err == nil {
		t.Fatal("idempotent save hid corruption")
	}
	if err = os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(filepath.Join(work, "file"), file); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Load(ctx, snap.Digest()); err == nil {
		t.Fatal("symlink followed")
	}
}
func TestStoreCancelledClosedAndInvalidID(t *testing.T) {
	work := t.TempDir()
	snap := capture(t, work)
	s, _ := newStore(t, work, DefaultStoreLimits())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Save(ctx, snap); err == nil {
		t.Fatal("cancel ignored")
	}
	ids, err := s.List()
	if err != nil || len(ids) != 0 {
		t.Fatal(ids, err)
	}
	if _, err = s.Load(context.Background(), "../escape"); err == nil {
		t.Fatal("invalid id accepted")
	}
	s.Close()
	if err = s.Save(context.Background(), snap); err == nil {
		t.Fatal("closed save accepted")
	}
}

func TestStoreRecoversUnpublishedWrite(t *testing.T) {
	work := t.TempDir()
	put(t, work, "file", "keep", 0600)
	snap := capture(t, work)
	s, dir := newStore(t, work, DefaultStoreLimits())
	if err := s.Save(context.Background(), snap); err != nil {
		t.Fatal(err)
	}
	s.Close()
	pending := filepath.Join(dir, ".pending-"+strings.Repeat("a", 32))
	if err := os.WriteFile(pending, []byte("incomplete JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(dir, work, DefaultStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err = os.Stat(pending); !os.IsNotExist(err) {
		t.Fatal("pending write survived recovery", err)
	}
	if _, err = reopened.Load(context.Background(), snap.Digest()); err != nil {
		t.Fatal("accepted snapshot lost", err)
	}
}
func TestStoreUnsafePendingNotRemoved(t *testing.T) {
	work := t.TempDir()
	s, dir := newStore(t, work, DefaultStoreLimits())
	s.Close()
	put(t, work, "sentinel", "keep", 0600)
	pending := filepath.Join(dir, ".pending-"+strings.Repeat("b", 32))
	if err := os.Symlink(filepath.Join(work, "sentinel"), pending); err != nil {
		t.Fatal(err)
	}
	if reopened, err := OpenStore(dir, work, DefaultStoreLimits()); err == nil {
		reopened.Close()
		t.Fatal("unsafe recovery accepted")
	}
	if _, err := os.Lstat(pending); err != nil {
		t.Fatal("unsafe path removed", err)
	}
	b, _ := os.ReadFile(filepath.Join(work, "sentinel"))
	if string(b) != "keep" {
		t.Fatal("sentinel modified")
	}
}
