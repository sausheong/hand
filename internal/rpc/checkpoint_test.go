package rpc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/checkpoints"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func TestCheckpointChangesRPC(t *testing.T) {
	ctx := context.Background()
	work := t.TempDir()
	sessions := session.NewStore(t.TempDir())
	if err := sessions.Create("hand", "key"); err != nil {
		t.Fatal(err)
	}
	sess, err := sessions.LoadExclusive("hand", "key")
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	rt := &runtime.Runtime{Session: sess}
	owner := app.New(&app.HarnessBackend{Runtime: rt}, app.Options{SessionID: "key"})
	store, err := checkpoints.OpenStore(filepath.Join(t.TempDir(), "checkpoints"), work, checkpoints.DefaultStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	boundary := &app.WorkspaceCheckpoints{Workspace: work, Limits: checkpoints.DefaultLimits(), Store: store}
	if err = owner.ConfigureCheckpoints(boundary); err != nil {
		t.Fatal(err)
	}
	finish, err := boundary.Begin(ctx, "rpc-run")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(work, "changed"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = finish(ctx); err != nil {
		t.Fatal(err)
	}
	ledgerFile := ledgerPath(t)
	ledger, err := OpenLedger(ledgerFile)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	d := NewControllerDispatcher(&app.Controller{Rt: rt, Owner: owner}, ledger)
	d.Dispatch(ctx, rpcRequest("hello", "hello", `{}`))
	response := d.Dispatch(ctx, rpcRequest("changes", "checkpoint.changes", `{}`))
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var page app.CheckpointChangesPage
	if err = json.Unmarshal(response.Result, &page); err != nil || page.Total != 1 || page.Changes[0].Path != "changed" {
		t.Fatal(page, err)
	}
	response = d.Dispatch(ctx, rpcRequest("preview", "checkpoint.restore_preview", `{"run_id":"rpc-run","paths":["changed"]}`))
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var preview app.CheckpointRestorePreview
	if err = json.Unmarshal(response.Result, &preview); err != nil || preview.Conflicts || len(preview.Actions) != 1 || preview.Actions[0].Operation != "remove" {
		t.Fatal(preview, err)
	}
	params, err := json.Marshal(map[string]any{"confirmed": false, "preview": preview})
	if err != nil {
		t.Fatal(err)
	}
	refused := d.Dispatch(ctx, rpcRequest("not-confirmed", "checkpoint.restore", string(params)))
	if refused.Error == nil || refused.Error.Code != "confirmation_required" {
		t.Fatal(refused)
	}
	if _, err := os.Stat(filepath.Join(work, "changed")); err != nil {
		t.Fatal("unconfirmed restore mutated file", err)
	}
	params, err = json.Marshal(map[string]any{"confirmed": true, "preview": preview})
	if err != nil {
		t.Fatal(err)
	}
	request := rpcRequest("restore-once", "checkpoint.restore", string(params))
	first := d.Dispatch(ctx, request)
	var outcome struct {
		Completed bool                         `json:"completed"`
		Files     []checkpoints.AppliedRestore `json:"files"`
	}
	if first.Error != nil {
		t.Fatal(first.Error)
	}
	if err = json.Unmarshal(first.Result, &outcome); err != nil || !outcome.Completed || len(outcome.Files) != 1 || !outcome.Files[0].Applied {
		t.Fatal(outcome, err)
	}
	if _, err := os.Stat(filepath.Join(work, "changed")); !os.IsNotExist(err) {
		t.Fatal("restore did not remove selected file", err)
	}
	recoveryResponse := d.Dispatch(ctx, rpcRequest("recoveries", "checkpoint.recoveries", `{}`))
	var recoveryPage app.CheckpointRecoveryPage
	if recoveryResponse.Error != nil {
		t.Fatal(recoveryResponse.Error)
	}
	if err = json.Unmarshal(recoveryResponse.Result, &recoveryPage); err != nil || recoveryPage.Total != 1 || recoveryPage.Recoveries[0].State != "applied" {
		t.Fatal(recoveryPage, err)
	}
	resolutionParams, err := json.Marshal(map[string]any{"confirmed": true, "action": "acknowledge", "recovery": recoveryPage.Recoveries[0]})
	if err != nil {
		t.Fatal(err)
	}
	resolutionRequest := rpcRequest("resolve-once", "checkpoint.recovery_resolve", string(resolutionParams))
	resolution := d.Dispatch(ctx, resolutionRequest)
	if resolution.Error != nil {
		t.Fatal(resolution.Error)
	}
	if err = os.WriteFile(filepath.Join(work, "changed"), []byte("new user bytes after restore"), 0600); err != nil {
		t.Fatal(err)
	}
	replay := d.Dispatch(ctx, request)
	if replay.Error != nil || string(replay.Result) != string(first.Result) {
		t.Fatal(replay)
	}
	ledger.Close()
	ledger, err = OpenLedger(ledgerFile)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	d2 := NewControllerDispatcher(&app.Controller{Rt: rt, Owner: owner}, ledger)
	d2.Dispatch(ctx, rpcRequest("hello-again", "hello", `{}`))
	replay = d2.Dispatch(ctx, request)
	if replay.Error != nil || string(replay.Result) != string(first.Result) {
		t.Fatal("restart replay changed", replay)
	}
	resolutionReplay := d2.Dispatch(ctx, resolutionRequest)
	if resolutionReplay.Error != nil || string(resolutionReplay.Result) != string(resolution.Result) {
		t.Fatal("resolution replay lost", resolutionReplay)
	}
	data, err := os.ReadFile(filepath.Join(work, "changed"))
	if err != nil || string(data) != "new user bytes after restore" {
		t.Fatal("replay mutated later file", string(data), err)
	}
	if changed := d2.Dispatch(ctx, rpcRequest("restore-once", "checkpoint.restore", `{"confirmed":false}`)); changed.Error == nil {
		t.Fatal("changed request payload reused")
	}
	response = d.Dispatch(ctx, rpcRequest("bad", "checkpoint.changes", `{"offset":-1}`))
	if response.Error == nil {
		t.Fatal("invalid offset accepted")
	}
}
