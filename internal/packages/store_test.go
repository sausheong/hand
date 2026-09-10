package packages

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func approveChange(t *testing.T, s *Store, r ChangeReview) ChangeResult {
	t.Helper()
	approval, err := r.ApprovalDigest()
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Apply(context.Background(), r, approval)
	if err != nil || !result.Committed {
		t.Fatal("apply", result, err)
	}
	return result
}
func TestPackageStoreInstallUpdateRollbackRemoveAndReopen(t *testing.T) {
	ctx := context.Background()
	dir := privateStageParent(t)
	source := writeFixture(t)
	s, err := OpenStore(dir, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if other, err := OpenStore(dir, "1.0.0"); !errors.Is(err, ErrStoreBusy) || other != nil {
		t.Fatal("second writer admitted", err)
	}
	_, pin, err := VerifyDirectory(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.PrepareInstall(ctx, source, pin)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := s.Apply(ctx, first, ""); err == nil || result.Committed {
		t.Fatal("unapproved install committed")
	}
	state, err := s.List()
	if err != nil || len(state.Packages) != 0 {
		t.Fatal("approval refusal modified store")
	}
	approveChange(t, s, first)
	approval, _ := first.ApprovalDigest()
	if result, err := s.Apply(ctx, first, approval); !errors.Is(err, ErrStaleReview) || result.Committed {
		t.Fatal("stale approval applied", result, err)
	}
	// Updating capabilities changes the pin and requires a distinct review.
	raw, err := os.ReadFile(filepath.Join(source, ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	m, err := DecodeManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	m.Version = "1.1.0"
	m.Extensions[0].Capabilities = append(m.Extensions[0].Capabilities, "file.read")
	raw, _ = json.Marshal(m)
	if err = os.WriteFile(filepath.Join(source, ManifestName), raw, 0600); err != nil {
		t.Fatal(err)
	}
	_, nextPin, err := VerifyDirectory(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	next, err := s.PrepareInstall(ctx, source, nextPin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Apply(ctx, next, approval); err == nil {
		t.Fatal("old approval authorised capability change")
	}
	approveChange(t, s, next)
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(dir, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	state, err = s.List()
	if err != nil || state.Generation != 2 || len(state.Packages) != 1 || state.Packages[0].Current != nextPin || len(state.Packages[0].Revisions) != 2 {
		t.Fatal("reopen lost revision history", state, err)
	}
	rollback, err := s.PrepareRollback(ctx, m.Name, pin)
	if err != nil {
		t.Fatal(err)
	}
	approveChange(t, s, rollback)
	state, err = s.List()
	if err != nil || state.Packages[0].Current != pin {
		t.Fatal("rollback lost exact pin", state, err)
	}
	if _, got, err := VerifyDirectory(ctx, filepath.Join(dir, "objects", pin)); err != nil || got != pin {
		t.Fatal("retained content invalid", got, err)
	}
	remove, err := s.PrepareRemove(m.Name)
	if err != nil {
		t.Fatal(err)
	}
	approveChange(t, s, remove)
	state, err = s.List()
	if err != nil || len(state.Packages) != 0 || state.Generation != 4 {
		t.Fatal("remove failed", state, err)
	}
	if _, _, err = VerifyDirectory(ctx, source); err != nil {
		t.Fatal("store modified source", err)
	}
}
func TestPackageStoreRejectsChangedSourceAndCorruptRetainedObject(t *testing.T) {
	ctx := context.Background()
	dir := privateStageParent(t)
	source := writeFixture(t)
	s, err := OpenStore(dir, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, pin, err := VerifyDirectory(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	review, err := s.PrepareInstall(ctx, source, pin)
	if err != nil {
		t.Fatal(err)
	}
	approval, _ := review.ApprovalDigest()
	file := filepath.Join(source, "note.py")
	original, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(file, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if result, err := s.Apply(ctx, review, approval); err == nil || result.Committed {
		t.Fatal("changed content committed")
	}
	state, err := s.List()
	if err != nil || state.Generation != 0 {
		t.Fatal("failed apply advanced generation")
	}
	if err = os.WriteFile(file, original, 0600); err != nil {
		t.Fatal(err)
	}
	approveChange(t, s, review)
	retained := filepath.Join(dir, "objects", pin, "note.py")
	if err = os.Chmod(retained, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(retained, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PrepareRollback(ctx, review.Name, pin); err == nil {
		t.Fatal("corrupt rollback review accepted")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "lock.json"), []byte(`{"schema":1,"schema":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if invalid, err := OpenStore(dir, "1.0.0"); err == nil || invalid != nil {
		t.Fatal("corrupt lock accepted")
	}
}
func TestPackageStoreReviewBindsDestinationAndGeneration(t *testing.T) {
	ctx := context.Background()
	source := writeFixture(t)
	_, pin, err := VerifyDirectory(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	a, err := OpenStore(privateStageParent(t), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := OpenStore(privateStageParent(t), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	review, err := a.PrepareInstall(ctx, source, pin)
	if err != nil {
		t.Fatal(err)
	}
	approval, _ := review.ApprovalDigest()
	if _, err = b.Apply(ctx, review, approval); err == nil {
		t.Fatal("cross-store approval replay accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if result, err := a.Apply(cancelled, review, approval); !errors.Is(err, context.Canceled) || result.Committed {
		t.Fatal("cancelled change committed")
	}
	review.Digest = strings.Repeat("0", 64)
	if _, err = review.ApprovalDigest(); err == nil {
		t.Fatal("review manifest/pin mismatch accepted")
	}
}

type cancelBeforeLockRename struct {
	context.Context
	cancel    context.CancelFunc
	directory string
}

func (c *cancelBeforeLockRename) Err() error {
	entries, _ := os.ReadDir(c.directory)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "lock-") && strings.HasSuffix(entry.Name(), ".tmp") {
			c.cancel()
		}
	}
	return c.Context.Err()
}
func TestPackageStoreCancellationBeforeRenamePreservesCommittedLock(t *testing.T) {
	ctx := context.Background()
	source := writeFixture(t)
	dir := privateStageParent(t)
	s, err := OpenStore(dir, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, pin, err := VerifyDirectory(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	install, err := s.PrepareInstall(ctx, source, pin)
	if err != nil {
		t.Fatal(err)
	}
	approveChange(t, s, install)
	before, err := os.ReadFile(filepath.Join(dir, "lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	remove, err := s.PrepareRemove(install.Name)
	if err != nil {
		t.Fatal(err)
	}
	approval, _ := remove.ApprovalDigest()
	base, cancel := context.WithCancel(ctx)
	defer cancel()
	interrupted := &cancelBeforeLockRename{Context: base, cancel: cancel, directory: dir}
	result, err := s.Apply(interrupted, remove, approval)
	if !errors.Is(err, context.Canceled) || result.Committed {
		t.Fatal("interrupted lock committed", result, err)
	}
	after, err := os.ReadFile(filepath.Join(dir, "lock.json"))
	if err != nil || string(before) != string(after) {
		t.Fatal("interrupted lock changed durable state", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatal("temporary lock survived")
		}
	}
	// A cancelled, uncommitted change can be explicitly retried with its review.
	approveChange(t, s, remove)
}
