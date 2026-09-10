package rpc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func TestTimeBudgetRPCExplicitDecisionAndReplay(t *testing.T) {
	ctx := context.Background()
	c := &app.Controller{Owner: app.New(nil, app.Options{SessionID: "time"}), Rt: &runtime.Runtime{Session: session.NewSession("hand", "time")}}
	ledger, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	d := NewControllerDispatcher(c, ledger)
	defer d.Close()
	d.Dispatch(ctx, rpcRequest("hello", "hello", `{}`))
	read := func(id string) app.TimeBudgetView {
		t.Helper()
		response := d.Dispatch(ctx, rpcRequest(id, "budget.time", `{}`))
		var view app.TimeBudgetView
		if response.Error != nil || json.Unmarshal(response.Result, &view) != nil {
			t.Fatalf("view %+v", response)
		}
		return view
	}
	if read("initial").Configured {
		t.Fatal("time budget implicitly enabled")
	}
	refused := d.Dispatch(ctx, rpcRequest("unconfirmed", "budget.time.decide", `{"seconds":60}`))
	if refused.Error == nil || refused.Error.Code != "confirmation_required" {
		t.Fatal("unconfirmed allowance accepted")
	}
	first := rpcRequest("first", "budget.time.decide", `{"seconds":60,"confirmed":true}`)
	if r := d.Dispatch(ctx, first); r.Error != nil {
		t.Fatal(r.Error)
	}
	before := read("before")
	if r := d.Dispatch(ctx, rpcRequest("second", "budget.time.decide", `{"seconds":120,"confirmed":true}`)); r.Error != nil {
		t.Fatal(r.Error)
	}
	newer := read("newer")
	if !newer.Deadline.After(before.Deadline) || !newer.Configured || newer.Disclosure == "" {
		t.Fatal("new decision not installed")
	}
	if r := d.Dispatch(ctx, first); r.Error != nil {
		t.Fatal(r.Error)
	}
	after := read("after")
	if !after.Deadline.Equal(newer.Deadline) || !after.DecidedAt.Equal(newer.DecidedAt) {
		t.Fatal("replay reset deadline")
	}
	if r := d.Dispatch(ctx, rpcRequest("overflow", "budget.time.decide", `{"seconds":9223372036854775807,"confirmed":true}`)); r.Error == nil || r.Error.Code != "budget_rejected" {
		t.Fatal("overflow accepted")
	}
}
