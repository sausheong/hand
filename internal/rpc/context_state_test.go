package rpc

import (
	"context"
	"encoding/json"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"testing"
)

func TestContextStateRPCRevisionAndReplay(t *testing.T) {
	ctx := context.Background()
	rt := &runtime.Runtime{Session: session.NewSession("hand", "state")}
	ledger, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	d := NewControllerDispatcher(&app.Controller{Rt: rt, Owner: app.New(nil, app.Options{SessionID: "state"})}, ledger)
	defer d.Close()
	if hello := d.Dispatch(ctx, rpcRequest("hello", "hello", `{}`)); hello.Error != nil {
		t.Fatal(hello.Error)
	}
	set := rpcRequest("set", "context.state.replace", `{"revision":0,"items":[{"id":"decision","kind":"decision","text":"Use the stable API"}]}`)
	first := d.Dispatch(ctx, set)
	if first.Error != nil {
		t.Fatal(first.Error)
	}
	stale := d.Dispatch(ctx, rpcRequest("stale", "context.state.replace", `{"revision":0,"items":[]}`))
	if stale.Error == nil {
		t.Fatal("stale update accepted")
	}
	clear := d.Dispatch(ctx, rpcRequest("clear", "context.state.replace", `{"revision":1,"items":[]}`))
	if clear.Error != nil {
		t.Fatal(clear.Error)
	}
	replay := d.Dispatch(ctx, set)
	if replay.Error != nil || string(replay.Result) != string(first.Result) {
		t.Fatal("replay changed result")
	}
	read := d.Dispatch(ctx, rpcRequest("read", "context.state", `{}`))
	var state runtime.ContextState
	if read.Error != nil || json.Unmarshal(read.Result, &state) != nil || state.Revision != 2 || len(state.Items) != 0 {
		t.Fatalf("replay resurrected data: %+v", read)
	}
	for i, params := range []string{`{"items":[]}`, `{"revision":2}`, `{"revision":2,"items":null}`, `{"revision":2,"items":[],"extra":true}`} {
		bad := d.Dispatch(ctx, rpcRequest(string(rune('a'+i)), "context.state.replace", params))
		if bad.Error == nil {
			t.Fatal("invalid params accepted", params)
		}
	}
}
