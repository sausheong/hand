//go:build unix

package app

import (
	"context"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/checkpoints"
	"github.com/sausheong/hand/internal/config"
)

func TestVerificationOwnedCommandAndLaterEdit(t *testing.T) {
	ctx := context.Background()
	p := testProcesses(t)
	store, err := checkpoints.OpenStore(filepath.Join(t.TempDir(), "checkpoints"), p.workspace, checkpoints.DefaultStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	boundary := &WorkspaceCheckpoints{Workspace: p.workspace, Limits: checkpoints.DefaultLimits(), Store: store, Processes: p}
	owner := New(&backendFixture{}, Options{RunBoundary: boundary})
	c := &Controller{Owner: owner, Processes: p, OutputStore: p.store, Rt: &runtime.Runtime{Session: session.NewSession("hand", "verification-context")}}
	profile := VerificationProfile{Name: "test", Command: []string{"/bin/sh", "-c", "printf verified"}}
	_, release, err := owner.reserve(ctx, Running)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.RunVerification(ctx, profile); err != ErrBusy {
		t.Fatal("verification bypassed owner", err)
	}
	release()
	record, err := c.RunVerification(ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	if record.View().Stdout != "verified" || record.View().ExitCode != 0 {
		t.Fatal(record.View())
	}
	state, err := c.CheckVerification(ctx, profile, record)
	if err != nil || state.Status != "passed" {
		t.Fatal(state, err)
	}
	evidence := t.TempDir()
	if err := os.Chmod(evidence, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := config.VerificationConfig{MaxRecords: 1, MaxBytes: 1 << 20, Directory: evidence, Profiles: []config.VerificationProfile{profile}}
	if err := c.ConfigureVerification(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	views, err := c.VerificationProfiles(ctx)
	if err != nil || len(views) != 1 {
		t.Fatal(views, err)
	}
	if _, err := c.RunNamedVerification(ctx, "test", "wrong"); err == nil {
		t.Fatal("wrong confirmation accepted")
	}
	saved, err := c.RunNamedVerification(ctx, "test", views[0].Digest)
	if err != nil || saved.ID == "" || saved.Assessment.Status != "passed" {
		t.Fatal(saved, err)
	}
	stateBefore, contextErr := c.ContextState(ctx)
	if contextErr != nil || len(stateBefore.Items) != 1 || stateBefore.Items[0].Reference != "hand-verification:"+saved.ID || !strings.Contains(stateBefore.Items[0].Text, saved.Record.Before) {
		t.Fatal("saved evidence missing from context", stateBefore, contextErr)
	}
	full, err := c.RunNamedVerification(ctx, "test", views[0].Digest)
	if err == nil || full.ID != "" || full.Assessment.Status != "unverified" || full.Record.ExitCode != 0 {
		t.Fatal("full store reported accepted evidence", full, err)
	}
	stateAfter, contextErr := c.ContextState(ctx)
	if contextErr != nil || stateAfter.Revision != stateBefore.Revision {
		t.Fatal("failed save changed context", stateAfter, contextErr)
	}
	files, err := filepath.Glob(filepath.Join(evidence, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatal("retention changed accepted inventory", files, err)
	}
	views[0].Command[0] = "mutated"
	checked, err := c.CheckSavedVerification(ctx, "test", saved.ID)
	if err != nil || checked.Assessment.Status != "passed" {
		t.Fatal(checked, err)
	}
	if err := os.WriteFile(filepath.Join(p.workspace, "later"), []byte("user edit"), 0600); err != nil {
		t.Fatal(err)
	}
	checked, err = c.CheckSavedVerification(ctx, "test", saved.ID)
	if err != nil || checked.Assessment.Status != "stale" {
		t.Fatal(checked, err)
	}
	captured, contextErr := c.ContextState(ctx)
	if contextErr != nil || len(captured.Items) != 1 || !strings.Contains(captured.Items[0].Text, `"assessment_at_check":"stale"`) {
		t.Fatal("stale assessment not retained", captured, contextErr)
	}
	state, err = c.CheckVerification(ctx, profile, record)
	if err != nil || state.Status != "stale" {
		t.Fatal(state, err)
	}
	if err := store.Delete(record.View().Before); err != nil {
		t.Fatal(err)
	}
	state, err = c.CheckVerification(ctx, profile, record)
	if err != nil || state.Status != "unverified" {
		t.Fatal(state, err)
	}
}
