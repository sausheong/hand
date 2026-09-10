//go:build darwin || linux

package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/protocol"
	"github.com/sausheong/harness/budget"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func TestRunBudgetChargedRejectionsSurviveSessionReopen(t *testing.T) {
	ctx := context.Background()
	store := session.NewStore(t.TempDir())
	if err := store.Create("hand", "charged-run"); err != nil {
		t.Fatal(err)
	}
	sess, err := store.LoadExclusive("hand", "charged-run")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sess.Close() }()
	path := ledgerPath(t)
	ledger, err := OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { ledger.Close() }()
	makeDispatcher := func() (*Dispatcher, *app.Controller) {
		c := &app.Controller{Owner: app.New(nil, app.Options{SessionID: sess.ID}), Rt: &runtime.Runtime{Session: sess}}
		d := NewControllerDispatcher(c, ledger)
		if r := d.Dispatch(ctx, rpcRequest("h", "hello", `{}`)); r.Error != nil {
			t.Fatal(r.Error)
		}
		return d, c
	}
	d, c := makeDispatcher()
	defer func() { d.Close() }()
	call := func(id, method, params string) protocol.Response {
		t.Helper()
		r := d.Dispatch(ctx, rpcRequest(id, method, params))
		if r.Error != nil {
			t.Fatal(r.Error)
		}
		return r
	}
	call("tokens", "budget.run.tokens.decide", `{"id":"job","limit":1000,"confirmed":true}`)
	call("cost", "budget.run.cost.decide", `{"id":"job","currency":"USD","limit_nano":1000,"strict":true,"confirmed":true}`)
	call("select", "budget.run.select", `{"id":"job","confirmed":true}`)
	tokens, err := budget.OpenRunTokenLedger(sess, "job")
	if err != nil {
		t.Fatal(err)
	}
	cost, err := budget.OpenRunCostLedger(sess, "job")
	if err != nil {
		t.Fatal(err)
	}
	price := &budget.PriceSnapshot{Provider: "local", Model: "fixture", Destination: "fixture", Currency: "USD", Source: "test tariff", Version: "1", EffectiveAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour), FixedNano: 10, AllChargesBounded: true}
	req := llm.ChatRequest{Model: "fixture", Route: llm.CallRoute{Provider: "local", Destination: "fixture"}, MaxTokens: 5}
	for _, admit := range []llm.CallAdmission{tokens.Admission(func(llm.ChatRequest) (int64, error) { return 10, nil }), cost.Admission(func(llm.ChatRequest, llm.CallCategory) (*budget.PriceSnapshot, int64, error) { return price, 10, nil })} {
		if _, err := admit(ctx, req, llm.CallRetry, "reserved"); err != nil {
			t.Fatal(err)
		}
		settle, err := admit(ctx, req, llm.CallCompaction, "settled")
		if err != nil {
			t.Fatal(err)
		}
		if err := settle(llm.RequestUsage{ID: "settled", Status: "completed", Usage: &llm.Usage{InputTokens: 2, OutputTokens: 1}}); err != nil {
			t.Fatal(err)
		}
	}
	read := func() app.RunBudgetView {
		t.Helper()
		var view app.RunBudgetView
		if err := json.Unmarshal(call("view", "budget.run", `{}`).Result, &view); err != nil {
			t.Fatal(err)
		}
		return view
	}
	before := read()
	if !before.Selected || before.ID != "job" || before.Tokens.Committed != 18 || before.Cost.CommittedNano != 20 || before.Tokens.Uncertain != 1 || before.Cost.Uncertain != 1 {
		t.Fatalf("wrong charges: %+v", before)
	}
	bad := []struct{ method, body string }{
		{"budget.run.tokens.decide", `{"id":"job","limit":-1,"confirmed":true}`},
		{"budget.run.cost.decide", `{"id":"job","currency":"EUR","limit_nano":2000,"strict":true,"confirmed":true}`},
		{"budget.run.select", `{"id":"missing","confirmed":true}`},
	}
	responses := make([]protocol.Response, len(bad))
	for i, tc := range bad {
		responses[i] = d.Dispatch(ctx, rpcRequest(fmt.Sprintf("bad-%d", i), tc.method, tc.body))
		if responses[i].Error == nil || responses[i].Error.Code != "budget_rejected" {
			t.Fatalf("wrong rejection: %+v", responses[i])
		}
	}
	if !reflect.DeepEqual(before, read()) {
		t.Fatal("rejection changed charged run")
	}
	d.Close()
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	sess, err = store.LoadExclusive("hand", "charged-run")
	if err != nil {
		t.Fatal(err)
	}
	ledger, err = OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	d, c = makeDispatcher()
	if !reflect.DeepEqual(before, read()) {
		t.Fatal("reopen changed selection, charges or reservations")
	}
	for i, tc := range bad {
		r := d.Dispatch(ctx, rpcRequest(fmt.Sprintf("bad-%d", i), tc.method, tc.body))
		if !reflect.DeepEqual(responses[i], r) {
			t.Fatal("persisted rejection changed on replay")
		}
	}
	if !reflect.DeepEqual(before, read()) {
		t.Fatal("replay reset charged run")
	}
	if err := c.DecideRunTokenBudget(ctx, "job", 2000); err != nil {
		t.Fatal(err)
	}
	after := read()
	if after.Tokens.Limit != 2000 || after.Tokens.Committed != 18 || after.Tokens.Uncertain != 1 || !reflect.DeepEqual(after.Cost, before.Cost) {
		t.Fatal("explicit ceiling increase erased prior usage")
	}
}
