//go:build darwin || linux

package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sausheong/harness/budget"
	"github.com/sausheong/harness/llm"
	"reflect"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func TestBudgetRPCRejectionsPreserveDecisionsAndReplay(t *testing.T) {
	ctx := context.Background()
	sess := session.NewSession("hand", "budget-rejections")
	backend := &rpcBackend{joined: make(chan struct{})}
	c := &app.Controller{Owner: app.New(backend, app.Options{SessionID: sess.ID}), Rt: &runtime.Runtime{Session: sess}}
	l, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	d := NewControllerDispatcher(c, l)
	defer d.Close()
	if r := d.Dispatch(ctx, rpcRequest("hello", "hello", `{}`)); r.Error != nil {
		t.Fatal(r.Error)
	}
	mutations := []struct{ method, body string }{
		{"budget.tokens.decide", `{"limit":1000}`},
		{"budget.cost.decide", `{"currency":"USD","limit_nano":1000,"strict":true}`},
		{"budget.time.decide", `{"seconds":600}`},
		{"budget.run.tokens.decide", `{"id":"job","limit":1000}`},
		{"budget.run.cost.decide", `{"id":"job","currency":"USD","limit_nano":1000,"strict":true}`},
		{"budget.run.time.decide", `{"id":"job","seconds":600}`},
		{"budget.run.select", `{"id":"job"}`},
		{"budget.prices.set", `{"prices":[]}`},
	}
	for i, m := range mutations {
		var params map[string]any
		if err := json.Unmarshal([]byte(m.body), &params); err != nil {
			t.Fatal(err)
		}
		params["confirmed"] = true
		raw, _ := json.Marshal(params)
		if r := d.Dispatch(ctx, rpcRequest(fmt.Sprintf("setup-%d", i), m.method, string(raw))); r.Error != nil {
			t.Fatal(r.Error)
		}
	}
	// Exercise refusal against actual recorded consumption, including an
	// outstanding reservation, rather than only unused budget ceilings.
	tokens, err := budget.OpenTokenLedger(sess)
	if err != nil {
		t.Fatal(err)
	}
	cost, err := budget.OpenCostLedger(sess)
	if err != nil {
		t.Fatal(err)
	}
	price := &budget.PriceSnapshot{Provider: "local", Model: "fixture", Destination: "fixture", Currency: "USD", Source: "test tariff", Version: "1", EffectiveAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour), FixedNano: 10, AllChargesBounded: true}
	request := llm.ChatRequest{Model: "fixture", Route: llm.CallRoute{Provider: "local", Destination: "fixture"}, MaxTokens: 5}
	admissions := []llm.CallAdmission{
		tokens.Admission(func(llm.ChatRequest) (int64, error) { return 10, nil }),
		cost.Admission(func(llm.ChatRequest, llm.CallCategory) (*budget.PriceSnapshot, int64, error) { return price, 10, nil }),
	}
	for _, admit := range admissions {
		if _, err := admit(ctx, request, llm.CallRetry, "outstanding"); err != nil {
			t.Fatal(err)
		}
		settle, err := admit(ctx, request, llm.CallCompaction, "settled")
		if err != nil {
			t.Fatal(err)
		}
		if err := settle(llm.RequestUsage{ID: "settled", Status: "completed", Usage: &llm.Usage{InputTokens: 2, OutputTokens: 1}}); err != nil {
			t.Fatal(err)
		}
	}
	tokenBefore, err := c.TokenBudget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	costBefore, err := c.CostBudget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if tokenBefore.Committed != 18 || tokenBefore.Attempts != 2 || tokenBefore.Uncertain != 1 || costBefore.CommittedNano != 20 || costBefore.Attempts != 2 || costBefore.Uncertain != 1 {
		t.Fatalf("missing charge fixture: %+v %+v", tokenBefore, costBefore)
	}
	before := sess.Entries()
	check := func(t *testing.T, id, method, body, code string) {
		t.Helper()
		request := rpcRequest(id, method, body)
		r := d.Dispatch(ctx, request)
		if r.Error == nil || r.Error.Code != code || r.RequestID != id || len(r.Result) != 0 {
			t.Fatalf("incorrect refusal: %+v", r)
		}
		if durableControl(method) {
			if replay := d.Dispatch(ctx, request); !reflect.DeepEqual(replay, r) {
				t.Fatalf("rejection changed on replay: %+v -> %+v", r, replay)
			}
		}
		if !reflect.DeepEqual(before, sess.Entries()) || backend.calls.Load() != 0 {
			t.Fatal("rejected budget request changed durable decisions or invoked model")
		}
		tokenAfter, err := c.TokenBudget(ctx)
		if err != nil {
			t.Fatal(err)
		}
		costAfter, err := c.CostBudget(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(tokenBefore, tokenAfter) || !reflect.DeepEqual(costBefore, costAfter) {
			t.Fatal("rejected budget request changed charges, reservations or uncertainty")
		}

	}
	for i, m := range mutations {
		t.Run(m.method, func(t *testing.T) {
			check(t, fmt.Sprintf("unconfirmed-%d", i), m.method, m.body, "confirmation_required")
			check(t, fmt.Sprintf("unknown-%d", i), m.method, `{"confirmed":true,"unexpected":1}`, "invalid_params")
			check(t, fmt.Sprintf("wrong-confirmation-%d", i), m.method, `{"confirmed":"true"}`, "invalid_params")
		})
	}
	for i, tc := range []struct{ method, body string }{
		{"budget.tokens.decide", `{"limit":-1,"confirmed":true}`},
		{"budget.cost.decide", `{"currency":"EUR","limit_nano":1000,"strict":true,"confirmed":true}`},
		{"budget.time.decide", `{"seconds":0,"confirmed":true}`},
		{"budget.run.tokens.decide", `{"id":"job","limit":-1,"confirmed":true}`},
		{"budget.run.cost.decide", `{"id":"job","currency":"EUR","limit_nano":1000,"strict":true,"confirmed":true}`},
		{"budget.run.time.decide", `{"id":"job","seconds":2592001,"confirmed":true}`},
		{"budget.run.select", `{"id":"missing","confirmed":true}`},
	} {
		check(t, fmt.Sprintf("invalid-decision-%d", i), tc.method, tc.body, "budget_rejected")
	}
	for i, method := range []string{"budget.tokens", "budget.cost", "budget.time", "budget.prices", "budget.run"} {
		check(t, fmt.Sprintf("invalid-read-%d", i), method, `{"unexpected":true}`, "invalid_params")
	}
}
