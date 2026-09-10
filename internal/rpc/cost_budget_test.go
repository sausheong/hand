package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/budget"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func TestCostBudgetRPCDecisionsPricesAndReplay(t *testing.T) {
	ctx := context.Background()
	c := &app.Controller{Owner: app.New(nil, app.Options{SessionID: "cost"}), Rt: &runtime.Runtime{Session: session.NewSession("hand", "cost")}}
	ledger, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	d := NewControllerDispatcher(c, ledger)
	defer d.Close()
	d.Dispatch(ctx, rpcRequest("hello", "hello", `{}`))
	first := rpcRequest("first", "budget.cost.decide", `{"currency":"USD","limit_nano":1000,"strict":true,"confirmed":true}`)
	if response := d.Dispatch(ctx, first); response.Error != nil {
		t.Fatal(response.Error)
	}
	for i, body := range []string{`{"currency":"USD","limit_nano":2000,"strict":true}`, `{"currency":"USD","limit_nano":2000,"confirmed":true}`, `{"currency":"EUR","limit_nano":2000,"strict":true,"confirmed":true}`} {
		// Unique IDs avoid conflating validation with conflicting replay.
		if response := d.Dispatch(ctx, rpcRequest(fmt.Sprintf("invalid-%d", i), "budget.cost.decide", body)); response.Error == nil || response.Error.Code != []string{"confirmation_required", "invalid_params", "budget_rejected"}[i] {
			t.Fatalf("incorrect refusal: %+v", response)
		}
	}
	if response := d.Dispatch(ctx, rpcRequest("second", "budget.cost.decide", `{"currency":"USD","limit_nano":2000,"strict":false,"confirmed":true}`)); response.Error != nil {
		t.Fatal(response.Error)
	}
	if response := d.Dispatch(ctx, first); response.Error != nil {
		t.Fatal(response.Error)
	}
	var view app.CostBudgetView
	response := d.Dispatch(ctx, rpcRequest("status", "budget.cost", `{}`))
	if response.Error != nil || json.Unmarshal(response.Result, &view) != nil || view.LimitNano != 2000 || view.Strict || view.Disclosure == "" {
		t.Fatalf("view %+v response %+v", view, response)
	}
	price := budget.PriceSnapshot{Provider: "local", Model: "fixture", Destination: "http://127.0.0.1/v1", Currency: "USD", Source: "local test tariff", Version: "1", EffectiveAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour), AllChargesBounded: true}
	payload := func(confirmed bool) string {
		raw, err := json.Marshal(map[string]any{"prices": []budget.PriceSnapshot{price}, "confirmed": confirmed})
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	if response := d.Dispatch(ctx, rpcRequest("unconfirmed-prices", "budget.prices.set", payload(false))); response.Error == nil || response.Error.Code != "confirmation_required" {
		t.Fatal("unconfirmed prices accepted")
	}
	install := rpcRequest("prices", "budget.prices.set", payload(true))
	if response := d.Dispatch(ctx, install); response.Error != nil {
		t.Fatal(response.Error)
	}
	if response := d.Dispatch(ctx, rpcRequest("clear", "budget.prices.set", `{"prices":[],"confirmed":true}`)); response.Error != nil {
		t.Fatal(response.Error)
	}
	if response := d.Dispatch(ctx, install); response.Error != nil {
		t.Fatal(response.Error)
	}
	response = d.Dispatch(ctx, rpcRequest("list", "budget.prices", `{}`))
	var prices []budget.PriceSnapshot
	if response.Error != nil || json.Unmarshal(response.Result, &prices) != nil || len(prices) != 0 {
		t.Fatal("replayed install overwrote cleared prices")
	}
}
