package packages

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func assertStoredOrigin(t *testing.T, s *Store, want Origin) {
	t.Helper()
	state, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Packages) != 1 || len(state.Packages[0].Revisions) != 1 {
		t.Fatal("unexpected stored inventory", state)
	}
	origin := state.Packages[0].Revisions[0].Origin
	if origin == nil || *origin != want {
		t.Fatalf("origin lost after snapshot removal: %+v, want %+v", origin, want)
	}
}
func TestOriginReviewApprovalAndLegacyMetadata(t *testing.T) {
	ctx := context.Background()
	source := writeFixture(t)
	_, pin, err := VerifyDirectory(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := StageDirectory(ctx, source, privateStageParent(t), pin)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	dir := privateStageParent(t)
	store, err := OpenStore(dir, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	review, err := store.PrepareSnapshot(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	approval, _ := review.ApprovalDigest()
	original := *review.Origin
	review.Origin.Location = t.TempDir()
	if result, err := store.Apply(ctx, review, approval); err == nil || result.Committed {
		t.Fatal("changed origin reused approval")
	}
	review.Origin = &original
	approveChange(t, store, review)
	copy := snapshot.Origin()
	copy.Location = "changed"
	if snapshot.Origin() != original {
		t.Fatal("origin getter leaked mutable state")
	}
	if err = snapshot.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PrepareSnapshot(ctx, snapshot); err == nil {
		t.Fatal("closed snapshot review accepted")
	}
	assertStoredOrigin(t, store, original)
	state, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	state.Packages[0].Revisions[0].Origin = nil
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "lock.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(dir, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	rollback, err := reopened.PrepareRollback(ctx, review.Name, pin)
	if err != nil {
		t.Fatal(err)
	}
	if rollback.Origin != nil {
		t.Fatal("legacy provenance was invented")
	}
	approveChange(t, reopened, rollback)
	state, err = reopened.List()
	if err != nil || state.Packages[0].Revisions[0].Origin != nil {
		t.Fatal("legacy metadata changed", err)
	}
}
func TestOriginValidation(t *testing.T) {
	for _, origin := range []Origin{{Kind: "local", Location: "relative"}, {Kind: "archive", Location: "/archive", Reference: "bad"}, {Kind: "git", Location: "/repo", Reference: "HEAD"}, {Kind: "local", Location: "/source", Reference: "unexpected"}, {Kind: "remote", Location: "/source"}} {
		if err := origin.Validate(); err == nil {
			t.Fatal("invalid origin accepted", origin)
		}
	}
}
