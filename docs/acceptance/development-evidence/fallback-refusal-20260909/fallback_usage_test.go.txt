package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/sausheong/harness/budget"
	"reflect"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
)

type fallbackUsageProvider struct {
	persistedUsageProvider
	models []string
}

func (p *fallbackUsageProvider) ChatStream(ctx context.Context, request llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	p.models = append(p.models, request.Model)
	if len(p.models) == 1 {
		return nil, errors.New("529 overloaded")
	}
	return p.persistedUsageProvider.ChatStream(ctx, request)
}

func TestFallbackUsageRetainsUnknownAttemptAndCacheSubtotalAfterReopen(t *testing.T) {
	store := session.NewStore(t.TempDir())
	if err := store.Create("hand", "key"); err != nil {
		t.Fatal(err)
	}
	sess, err := store.LoadExclusive("hand", "key")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if sess != nil {
			sess.Close()
		}
	}()
	provider := &fallbackUsageProvider{}
	backend := &HarnessBackend{Runtime: &runtime.Runtime{LLM: provider, Session: sess, Tools: tool.NewRegistry(), AgentID: "hand", Model: "primary", FallbackModel: "fallback", MaxTurns: 2}}

	backend.Runtime.Route = llm.CallRoute{Provider: "local", Destination: "fixture"}
	controller := &Controller{Rt: backend.Runtime}
	if err := controller.DecideTokenBudget(context.Background(), 100000); err != nil {
		t.Fatal(err)
	}
	if err := controller.DecideCostBudget(context.Background(), "USD", 1000, true); err != nil {
		t.Fatal(err)
	}
	var prices []budget.PriceSnapshot
	for _, model := range []string{"primary", "fallback"} {
		prices = append(prices, budget.PriceSnapshot{Provider: "local", Destination: "fixture", Model: model, Currency: "USD", Source: "deterministic test tariff", Version: "1", EffectiveAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour), FixedNano: 100, AllChargesBounded: true})
	}
	if err := controller.SetCostPrices(context.Background(), prices); err != nil {
		t.Fatal(err)
	}
	var attempts []llm.RequestUsage
	ctx := llm.WithUsageObserver(context.Background(), func(record llm.RequestUsage) { attempts = append(attempts, record) })
	stream, err := backend.Run(ctx, "go", nil)
	if err != nil {
		t.Fatal(err)
	}
	var saved Details
	summaries := 0
	for event := range stream {
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		if event.Kind == "session_usage" {
			saved = event.Details
			summaries++
		}
	}
	if !reflect.DeepEqual(provider.models, []string{"primary", "fallback"}) || len(attempts) != 2 {
		t.Fatal("fallback attempts lost", provider.models, attempts)
	}
	if attempts[0].Model != "primary" || attempts[0].Status != "failed" || attempts[0].Usage != nil || attempts[0].Source != "unavailable" {
		t.Fatal("failed attempt became known", attempts[0])
	}
	if attempts[1].Model != "fallback" || attempts[1].Category != llm.CallRetry || attempts[1].Status != "completed" || attempts[1].ID == attempts[0].ID {
		t.Fatal("fallback provenance lost", attempts[1])
	}
	if summaries != 1 || saved.InputTokens != 40 || saved.OutputTokens != 5 || saved.CacheReadInputTokens != 20 || saved.UsageRequests != 2 || saved.UsageUnknown != 1 || saved.UsagePriorUnknown {
		t.Fatal("incorrect accounting snapshot", saved)
	}
	wire, err := NewWireEvents("request").Encode(Event{Kind: "terminal", Details: saved})
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Usage struct {
			Known   bool `json:"known"`
			Input   int  `json:"input_tokens"`
			Unknown int  `json:"unknown_requests"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(wire.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Usage.Known || payload.Usage.Input != 40 || payload.Usage.Unknown != 1 {
		t.Fatal("wire hid uncertainty or counted cache twice", string(wire.Payload))
	}

	tokenBefore, err := controller.TokenBudget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	costBefore, err := controller.CostBudget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tokenState, err := backend.Runtime.TokenBudget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tokenState.Attempts) != 2 || tokenState.Attempts[0].Charged != tokenState.Attempts[0].Reserved || tokenState.Attempts[0].Reserved <= 0 || tokenState.Attempts[1].Charged != 45 {
		t.Fatal("fallback erased reservation or charged cache twice", tokenState)
	}
	if tokenBefore.Attempts != 2 || tokenBefore.Uncertain != 1 || tokenBefore.Committed != tokenState.Attempts[0].Reserved+45 {
		t.Fatal("incorrect token commitment", tokenBefore)
	}
	if costBefore.Attempts != 2 || costBefore.Uncertain != 1 || costBefore.Unpriced != 0 || costBefore.CommittedNano != 200 || !costBefore.Strict {
		t.Fatal("unknown attempt became free", costBefore)
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	sess, err = store.LoadExclusive("hand", "key")
	if err != nil {
		t.Fatal(err)
	}

	controller = &Controller{Rt: &runtime.Runtime{Session: sess}}
	tokenAfter, err := controller.TokenBudget(context.Background())
	if err != nil || !reflect.DeepEqual(tokenBefore, tokenAfter) {
		t.Fatal("reopen changed token commitment", tokenAfter, err)
	}
	costAfter, err := controller.CostBudget(context.Background())
	if err != nil || !reflect.DeepEqual(costBefore, costAfter) {
		t.Fatal("reopen changed cost commitment", costAfter, err)
	}
	summary, err := sessionio.ReadUsage(sess)
	if err != nil || summary.Requests != 2 || summary.Unknown != 1 || summary.Total != (llm.Usage{InputTokens: 40, OutputTokens: 5, CacheReadInputTokens: 20}) || summary.PriorUsageUnknown {
		t.Fatal("reopen changed accounting", summary, err)
	}
	for _, attempt := range attempts {
		if err := sessionio.RecordRequestUsage(sess, attempt); err != nil {
			t.Fatal(err)
		}
	}
	replayed, err := sessionio.ReadUsage(sess)
	if err != nil || !reflect.DeepEqual(summary, replayed) {
		t.Fatal("duplicate observation charged twice", replayed, err)
	}
}

func TestFallbackCostAdmissionRefusesBeforeSecondProviderCall(t *testing.T) {
	for _, reason := range []string{"missing tariff", "expired tariff", "cost ceiling"} {
		t.Run(reason, func(t *testing.T) {
			sess := session.NewSession("hand", "key")
			defer sess.Close()
			provider := &fallbackUsageProvider{}
			rt := &runtime.Runtime{LLM: provider, Session: sess, Tools: tool.NewRegistry(), AgentID: "hand", Model: "primary", FallbackModel: "fallback", MaxTurns: 2, Route: llm.CallRoute{Provider: "local", Destination: "fixture"}}
			controller := &Controller{Rt: rt}
			limit := int64(1000)
			if reason == "cost ceiling" {
				limit = 100
			}
			if err := controller.DecideCostBudget(context.Background(), "USD", limit, true); err != nil {
				t.Fatal(err)
			}
			price := budget.PriceSnapshot{Provider: "local", Destination: "fixture", Model: "primary", Currency: "USD", Source: "deterministic test tariff", Version: "1", EffectiveAt: time.Now().Add(-2 * time.Hour), ExpiresAt: time.Now().Add(time.Hour), FixedNano: 100, AllChargesBounded: true}
			prices := []budget.PriceSnapshot{price}
			if reason != "missing tariff" {
				price.Model = "fallback"
				if reason == "expired tariff" {
					price.ExpiresAt = time.Now().Add(-time.Hour)
				}
				prices = append(prices, price)
			}
			if err := controller.SetCostPrices(context.Background(), prices); err != nil {
				t.Fatal(err)
			}
			backend := &HarnessBackend{Runtime: rt}
			events, err := backend.Run(context.Background(), "go", nil)
			if err != nil {
				t.Fatal(err)
			}
			var failures []error
			summaries := 0
			for event := range events {
				if event.Err != nil {
					failures = append(failures, event.Err)
				}
				if event.Kind == "session_usage" {
					summaries++
					if event.Details.UsageRequests != 1 || event.Details.UsageUnknown != 1 {
						t.Fatal("refused attempt counted as provider call", event)
					}
				}
			}
			if !reflect.DeepEqual(provider.models, []string{"primary"}) || len(failures) == 0 || summaries != 1 {
				t.Fatal("fallback was not refused before provider dispatch", provider.models, failures, summaries)
			}
			view, err := controller.CostBudget(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if view.Attempts != 1 || view.CommittedNano != 100 || view.Uncertain != 1 || view.Unpriced != 0 || !view.Strict {
				t.Fatal("rejection changed first attempt charge", view)
			}
		})
	}
}
