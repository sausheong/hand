package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/sausheong/hand/internal/checkpoints"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func TestCheckpointChangesDurablePagingAndMissingEvidence(t *testing.T) {
	ctx := context.Background()
	work := t.TempDir()
	ss := session.NewStore(t.TempDir())
	if err := ss.Create("hand", "key"); err != nil {
		t.Fatal(err)
	}
	sess, err := ss.LoadExclusive("hand", "key")
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	rt := &runtime.Runtime{Session: sess}
	backend := &HarnessBackend{Runtime: rt}
	owner := New(backend, Options{})
	controller := &Controller{Owner: owner, Rt: rt}
	store, err := checkpoints.OpenStore(filepath.Join(t.TempDir(), "store"), work, checkpoints.DefaultStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	boundary := &WorkspaceCheckpoints{Workspace: work, Store: store, Limits: checkpoints.DefaultLimits()}
	if err = owner.ConfigureCheckpoints(boundary); err != nil {
		t.Fatal(err)
	}
	finish, err := boundary.Begin(ctx, "run")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 35; i++ {
		if err = os.WriteFile(filepath.Join(work, fmt.Sprintf("file-%02d", i)), []byte("created"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err = finish(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := controller.CheckpointChanges(ctx, "", 0)
	if err != nil || len(page.Changes) != 32 || page.Total != 35 || page.Next != 32 {
		t.Fatal(page, err)
	}
	next, err := controller.CheckpointChanges(ctx, page.RunID, page.Next)
	if err != nil || len(next.Changes) != 3 || next.Next != 35 {
		t.Fatal(next, err)
	}
	if page.Changes[0].Before != nil || page.Changes[0].After == nil {
		t.Fatal("wrong change kind")
	}
	_, release, err := owner.reserve(ctx, Running)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = controller.CheckpointChanges(ctx, "", 0); err != ErrBusy {
		t.Fatal("inspection bypassed ownership", err)
	}
	release()
	preview, err := controller.PreviewCheckpointRestore(ctx, "run", []string{"file-00"})
	if err != nil || preview.Conflicts || preview.Actions[0].Operation != "remove" {
		t.Fatal(preview, err)
	}
	if err = os.WriteFile(filepath.Join(work, "file-00"), []byte("later user edit"), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := controller.PreviewCheckpointRestore(ctx, "run", []string{"file-00"})
	if err != nil || !changed.Conflicts || changed.Current == preview.Current {
		t.Fatal(changed, err)
	}
	data, _ := os.ReadFile(filepath.Join(work, "file-00"))
	if string(data) != "later user edit" {
		t.Fatal("preview modified user edit")
	}
	if result, err := controller.ApplyCheckpointRestore(ctx, preview); err == nil || len(result) != 0 {
		t.Fatal("stale confirmation accepted", result, err)
	}
	valid, err := controller.PreviewCheckpointRestore(ctx, "run", []string{"file-01"})
	if err != nil || valid.Conflicts {
		t.Fatal(valid, err)
	}
	_, releaseRestore, err := owner.reserve(ctx, Running)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.ApplyCheckpointRestore(ctx, valid); err != ErrBusy {
		t.Fatal("restore bypassed run ownership", err)
	}
	releaseRestore()
	altered := valid
	altered.After = valid.Before
	if _, err := controller.ApplyCheckpointRestore(ctx, altered); err == nil {
		t.Fatal("altered run binding accepted")
	}
	results, err := controller.ApplyCheckpointRestore(ctx, valid)
	if err != nil || len(results) != 1 || !results[0].Applied {
		t.Fatal(results, err)
	}
	if _, err := os.Stat(filepath.Join(work, "file-01")); !os.IsNotExist(err) {
		t.Fatal("confirmed file not removed", err)
	}
	data, _ = os.ReadFile(filepath.Join(work, "file-00"))
	if string(data) != "later user edit" {
		t.Fatal("restore changed unselected user edit")
	}
	if _, err := controller.ApplyCheckpointRestore(ctx, valid); err == nil {
		t.Fatal("stale successful confirmation replayed")
	}
	prepared := checkpoints.RestoreEvent{Phase: "prepared", Path: "file-02", Operation: "remove", RecoveryName: ".hand-restore-00000000000000000000000000000001", Expected: page.Changes[2].After}
	if err = store.RecordRestore(prepared); err != nil {
		t.Fatal(err)
	}
	recoveries, err := controller.CheckpointRecoveries(ctx, 0)
	if err != nil || recoveries.Total != 2 || recoveries.Next != 2 || recoveries.Recoveries[1].State != "prepared" {
		t.Fatal(recoveries, err)
	}
	if _, err := controller.CheckpointRecoveries(ctx, 3); err == nil {
		t.Fatal("invalid recovery offset accepted")
	}
	_, releaseRecovery, err := owner.reserve(ctx, Running)
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.ResolveCheckpointRecovery(ctx, "cancel", recoveries.Recoveries[1]); err != ErrBusy {
		t.Fatal("recovery bypassed owner", err)
	}
	releaseRecovery()
	if err := controller.ResolveCheckpointRecovery(ctx, "discard", recoveries.Recoveries[1]); err == nil {
		t.Fatal("unknown recovery action accepted")
	}
	if err := controller.ResolveCheckpointRecovery(ctx, "acknowledge", recoveries.Recoveries[1]); err == nil {
		t.Fatal("prepared recovery acknowledged")
	}
	if err := controller.ResolveCheckpointRecovery(ctx, "cancel", recoveries.Recoveries[1]); err != nil {
		t.Fatal(err)
	}
	if err := controller.ResolveCheckpointRecovery(ctx, "cancel", recoveries.Recoveries[1]); err != nil {
		t.Fatal("retry", err)
	}
	resolved, err := controller.CheckpointRecoveries(ctx, 1)
	if err != nil || len(resolved.Recoveries) != 1 || resolved.Recoveries[0].State != "cancelled" {
		t.Fatal(resolved, err)
	}
	data, err = os.ReadFile(filepath.Join(work, "file-02"))
	if err != nil || string(data) != "created" {
		t.Fatal("cancellation changed target", string(data), err)
	}
	if err = store.Delete(page.After); err != nil {
		t.Fatal(err)
	}
	if _, err = controller.CheckpointChanges(ctx, "run", 0); err == nil {
		t.Fatal("missing checkpoint became empty diff")
	}
}
