package rpc

import (
	"context"
	"encoding/json"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"testing"
)

func TestTokenBudgetRPCExplicitDecisionAndReplay(t *testing.T) {
	ctx := context.Background()
	c := &app.Controller{Owner: app.New(nil, app.Options{SessionID: "budget"}), Rt: &runtime.Runtime{Session: session.NewSession("hand", "budget")}}
	ledger, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	d := NewControllerDispatcher(c, ledger)
	defer d.Close()
	d.Dispatch(ctx, rpcRequest("hello", "hello", `{}`))
	refused := d.Dispatch(ctx, rpcRequest("unconfirmed", "budget.tokens.decide", `{"limit":100}`))
	if refused.Error == nil || refused.Error.Code != "confirmation_required" {
		t.Fatal("unconfirmed decision accepted")
	}
	first := rpcRequest("decision", "budget.tokens.decide", `{"limit":100,"confirmed":true}`)
	if r := d.Dispatch(ctx, first); r.Error != nil {
		t.Fatal(r.Error)
	}
	if r := d.Dispatch(ctx, rpcRequest("new-decision", "budget.tokens.decide", `{"limit":200,"confirmed":true}`)); r.Error != nil {
		t.Fatal(r.Error)
	}
	if r := d.Dispatch(ctx, first); r.Error != nil {
		t.Fatal(r.Error)
	}
	response := d.Dispatch(ctx, rpcRequest("status", "budget.tokens", `{}`))
	var view app.TokenBudgetView
	if response.Error != nil || json.Unmarshal(response.Result, &view) != nil || view.Limit != 200 || view.Committed != 0 || view.EstimateMethod == "" {
		t.Fatalf("status %+v view %+v", response, view)
	}
	if r := d.Dispatch(ctx, rpcRequest("invalid", "budget.tokens.decide", `{"limit":-1,"confirmed":true}`)); r.Error == nil {
		t.Fatal("invalid ceiling accepted")
	}
}
