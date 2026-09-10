package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func TestRunBudgetRPCDecisionsSelectionAndReplay(t *testing.T) {
	ctx := context.Background()
	c := &app.Controller{Owner: app.New(nil, app.Options{SessionID: "run"}), Rt: &runtime.Runtime{Session: session.NewSession("hand", "run")}}
	ledger, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	d := NewControllerDispatcher(c, ledger)
	defer d.Close()
	d.Dispatch(ctx, rpcRequest("hello", "hello", `{}`))
	read := func(id string) app.RunBudgetView {
		t.Helper()
		response := d.Dispatch(ctx, rpcRequest(id, "budget.run", `{}`))
		var view app.RunBudgetView
		if response.Error != nil || json.Unmarshal(response.Result, &view) != nil {
			t.Fatalf("view %+v", response)
		}
		if len(response.Result) > 8192 {
			t.Fatal("run budget view exceeds bound")
		}
		return view
	}
	if read("initial").Selected {
		t.Fatal("implicit selection")
	}
	for i, tc := range []struct{ method, body, code string }{
		{"budget.run.tokens.decide", `{"id":"job","limit":100}`, "confirmation_required"},
		{"budget.run.cost.decide", `{"id":"job","currency":"USD","limit_nano":100,"confirmed":true}`, "invalid_params"},
		{"budget.run.select", `{"id":"missing","confirmed":true}`, "budget_rejected"},
		{"budget.run.time.decide", `{"id":"job","seconds":9223372036854775807,"confirmed":true}`, "budget_rejected"},
		{"budget.run", `{"limit":2}`, "invalid_params"},
	} {
		r := d.Dispatch(ctx, rpcRequest(fmt.Sprintf("bad-%d", i), tc.method, tc.body))
		if r.Error == nil || r.Error.Code != tc.code {
			t.Fatalf("invalid request %+v", r)
		}
	}
	first := rpcRequest("tokens-first", "budget.run.tokens.decide", `{"id":"job","limit":100,"confirmed":true}`)
	if r := d.Dispatch(ctx, first); r.Error != nil {
		t.Fatal(r.Error)
	}
	if r := d.Dispatch(ctx, rpcRequest("tokens-second", "budget.run.tokens.decide", `{"id":"job","limit":200,"confirmed":true}`)); r.Error != nil {
		t.Fatal(r.Error)
	}
	if r := d.Dispatch(ctx, first); r.Error != nil {
		t.Fatal(r.Error)
	}
	selected := rpcRequest("select-first", "budget.run.select", `{"id":"job","confirmed":true}`)
	if r := d.Dispatch(ctx, selected); r.Error != nil {
		t.Fatal(r.Error)
	}
	if view := read("job-status"); view.ID != "job" || !view.Selected || view.Tokens.Limit != 200 || !view.Configured {
		t.Fatalf("replayed decision changed state %+v", view)
	}
	deadline := rpcRequest("time-first", "budget.run.time.decide", `{"id":"job","seconds":60,"confirmed":true}`)
	if r := d.Dispatch(ctx, deadline); r.Error != nil {
		t.Fatal(r.Error)
	}
	if r := d.Dispatch(ctx, rpcRequest("time-second", "budget.run.time.decide", `{"id":"job","seconds":120,"confirmed":true}`)); r.Error != nil {
		t.Fatal(r.Error)
	}
	newer := read("time-newer")
	if r := d.Dispatch(ctx, deadline); r.Error != nil {
		t.Fatal(r.Error)
	}
	if view := read("time-after"); !view.Time.Deadline.Equal(newer.Time.Deadline) {
		t.Fatal("replay renewed deadline")
	}
	if r := d.Dispatch(ctx, rpcRequest("other-cost", "budget.run.cost.decide", `{"id":"other","currency":"USD","limit_nano":1000,"strict":true,"confirmed":true}`)); r.Error != nil {
		t.Fatal(r.Error)
	}
	if r := d.Dispatch(ctx, rpcRequest("select-other", "budget.run.select", `{"id":"other","confirmed":true}`)); r.Error != nil {
		t.Fatal(r.Error)
	}
	if r := d.Dispatch(ctx, selected); r.Error != nil {
		t.Fatal(r.Error)
	}
	if view := read("selected-after-replay"); view.ID != "other" || !view.Cost.Strict || view.Cost.LimitNano != 1000 {
		t.Fatalf("replay changed selection %+v", view)
	}
}
