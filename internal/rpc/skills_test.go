package rpc

import (
	"context"
	"encoding/json"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tool/skills"
	"strings"
	"testing"
)

func TestSkillsReloadRPCAndReplay(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	p, store, err := agentio.BuildSkillProvider(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rt, err := runtime.BuildRuntime(runtime.RuntimeDeps{Skills: p}, runtime.RuntimeInputs{Tools: tool.NewRegistry(), Session: session.NewSession("hand", "skills")}, runtime.AgentSpec{SystemPrompt: "identity"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	ledger, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	d := NewControllerDispatcher(&app.Controller{Rt: rt, Owner: app.New(nil, app.Options{SessionID: "skills"})}, ledger)
	defer d.Close()
	if hello := d.Dispatch(ctx, rpcRequest("hello", "hello", `{}`)); hello.Error != nil {
		t.Fatal(hello.Error)
	}
	if _, err = store.Create(ctx, skills.Skill{Name: "rpc-authored", Body: "body"}); err != nil {
		t.Fatal(err)
	}
	bad := d.Dispatch(ctx, rpcRequest("bad", "skills.reload", `{"extra":true}`))
	if bad.Error == nil {
		t.Fatal("unknown parameter accepted")
	}
	pinRequest := rpcRequest("pin", "context.pin", `{"id":"objective","kind":"objective","text":"ship working code"}`)
	pinned := d.Dispatch(ctx, pinRequest)
	if pinned.Error != nil {
		t.Fatal(pinned.Error)
	}
	removed := d.Dispatch(ctx, rpcRequest("unpin", "context.unpin", `{"id":"objective"}`))
	if removed.Error != nil {
		t.Fatal(removed.Error)
	}
	replayed := d.Dispatch(ctx, pinRequest)
	if replayed.Error != nil || string(replayed.Result) != string(pinned.Result) {
		t.Fatal("pin replay changed")
	}
	listed := d.Dispatch(ctx, rpcRequest("pins", "context.pins", `{}`))
	var pins []runtime.ContextPin
	if listed.Error != nil || json.Unmarshal(listed.Result, &pins) != nil || len(pins) != 0 {
		t.Fatal("replay recreated removed pin")
	}
	before := rt.StaticSystemPrompt
	inspected := d.Dispatch(ctx, rpcRequest("inspect", "context.inspect", `{}`))
	var report runtime.ContextInspection
	if inspected.Error != nil || json.Unmarshal(inspected.Result, &report) != nil || len(report.Contributions) != 4 || !strings.Contains(report.EstimateMethod, "not provider-reported") {
		t.Fatalf("inspection: %+v", inspected)
	}
	if rt.StaticSystemPrompt != before {
		t.Fatal("inspection implicitly refreshed skills")
	}
	if bad := d.Dispatch(ctx, rpcRequest("inspect-bad", "context.inspect", `{"extra":true}`)); bad.Error == nil {
		t.Fatal("invalid inspection accepted")
	}
	request := rpcRequest("reload", "skills.reload", `{}`)
	first := d.Dispatch(ctx, request)
	if first.Error != nil || !strings.Contains(rt.StaticSystemPrompt, "rpc-authored") {
		t.Fatalf("reload: %+v", first)
	}
	if err = store.Remove(ctx, "rpc-authored"); err != nil {
		t.Fatal(err)
	}
	again := d.Dispatch(ctx, request)
	if again.Error != nil || string(again.Result) != string(first.Result) || !strings.Contains(rt.StaticSystemPrompt, "rpc-authored") {
		t.Fatal("duplicate reload executed again")
	}
	fresh := d.Dispatch(ctx, rpcRequest("fresh", "skills.reload", `{}`))
	if fresh.Error != nil || strings.Contains(rt.StaticSystemPrompt, "rpc-authored") {
		t.Fatalf("fresh reload: %+v", fresh)
	}
}
