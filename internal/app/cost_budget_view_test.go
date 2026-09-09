package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sausheong/harness/budget"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func TestCostBudgetViewRetainsReservationsAndCompactionAfterRestart(t *testing.T) {
	ctx := context.Background()
	store := session.NewStore(t.TempDir())
	if err := store.Create("hand", "cost"); err != nil {
		t.Fatal(err)
	}
	s, err := store.LoadExclusive("hand", "cost")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	c := &Controller{Rt: &runtime.Runtime{Session: s}}
	if err := c.DecideCostBudget(ctx, "USD", 1000, false); err != nil {
		t.Fatal(err)
	}
	ledger, err := budget.OpenCostLedger(s)
	if err != nil {
		t.Fatal(err)
	}
	price := &budget.PriceSnapshot{Provider: "local", Model: "test", Destination: "fixture", Currency: "USD", Source: "test tariff", Version: "1", EffectiveAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour), FixedNano: 100, AllChargesBounded: true}
	resolve := func(llm.ChatRequest, llm.CallCategory) (*budget.PriceSnapshot, int64, error) { return price, 1, nil }
	req := llm.ChatRequest{Model: "test", Route: llm.CallRoute{Provider: "local", Destination: "fixture"}, MaxTokens: 1}
	if _, err := ledger.Admission(resolve)(ctx, req, llm.CallRetry, "outstanding"); err != nil {
		t.Fatal(err)
	}
	settle, err := ledger.Admission(resolve)(ctx, req, llm.CallCompaction, "summary")
	if err != nil {
		t.Fatal(err)
	}
	if err := settle(llm.RequestUsage{ID: "summary", Status: "completed", Usage: &llm.Usage{InputTokens: 1, OutputTokens: 1}}); err != nil {
		t.Fatal(err)
	}
	price = nil
	settle, err = ledger.Admission(resolve)(ctx, req, llm.CallGeneration, "unpriced")
	if err != nil {
		t.Fatal(err)
	}
	if err := settle(llm.RequestUsage{ID: "unpriced", Status: "completed", Usage: &llm.Usage{InputTokens: 1}}); err != nil {
		t.Fatal(err)
	}
	before, err := c.CostBudget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before.Currency != "USD" || before.LimitNano != 1000 || before.CommittedNano != 200 || before.CompactionNano != 100 || before.Attempts != 3 || before.Unpriced != 1 || before.Uncertain != 2 || before.Strict || before.Exhausted || !strings.Contains(before.Disclosure, "Unknown charges are not free") {
		t.Fatalf("incorrect cost disclosure: %+v", before)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = store.LoadExclusive("hand", "cost")
	if err != nil {
		t.Fatal(err)
	}
	c = &Controller{Rt: &runtime.Runtime{Session: s}}
	after, err := c.CostBudget(ctx)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("restart changed cost view: %+v %v", after, err)
	}
	if err := c.DecideCostBudget(ctx, "USD", 2000, true); err == nil {
		t.Fatal("strict mode erased historical unknown charge")
	}
	after, err = c.CostBudget(ctx)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("rejected strict decision changed budget")
	}
}

func TestCostBudgetViewFailuresReleaseOwnership(t *testing.T) {
	for _, failure := range []string{"runtime", "session", "cancelled", "busy"} {
		t.Run(failure, func(t *testing.T) {
			c := &Controller{Rt: &runtime.Runtime{Session: session.NewSession("hand", "cost")}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			release := func() {}
			switch failure {
			case "runtime":
				c.Rt = nil
			case "session":
				c.Rt.Session = nil
			case "cancelled":
				cancel()
			case "busy":
				var err error
				_, release, err = c.owner().reserve(ctx, Idle)
				if err != nil {
					t.Fatal(err)
				}
			}
			view, err := c.CostBudget(ctx)
			release()
			if err == nil || !reflect.DeepEqual(view, CostBudgetView{}) {
				t.Fatalf("failure returned usable budget: %+v %v", view, err)
			}
			if failure == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if failure == "busy" && !errors.Is(err, ErrBusy) {
				t.Fatal(err)
			}
			_, done, err := c.owner().reserve(context.Background(), Idle)
			if err != nil {
				t.Fatal("ownership leaked", err)
			}
			done()
		})
	}
}
