package app

import (
	"context"
	"fmt"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/budget"
	"github.com/sausheong/harness/runtime"
	"testing"
)

func TestBudgetExhaustionOutcomeSkipsVerification(t *testing.T) {
	for _, tc := range []struct {
		cause  error
		reason string
	}{
		{budget.ErrExhausted, "session_token_budget"},
		{runtime.ErrRunTokenExhausted, "run_token_budget"},
		{runtime.ErrRunCostExhausted, "run_cost_budget"},
		{budget.ErrRunTimeExhausted, "run_time_budget"},
		{budget.ErrTimeExhausted, "session_time_budget"},
		{budget.ErrCostExhausted, "session_cost_budget"},
		{budget.ErrUnknownPrice, "cost_price_unknown"},
	} {
		for _, start := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s_start_%v", tc.reason, start), func(t *testing.T) {
				cause := fmt.Errorf("provider admission: %w", tc.cause)
				backend := &backendFixture{run: func(context.Context, string) (<-chan BackendEvent, error) {
					if start {
						return nil, cause
					}
					ch := make(chan BackendEvent, 1)
					ch <- BackendEvent{Err: cause}
					close(ch)
					return ch, nil
				}}
				service := New(backend, Options{SessionID: "budget", MaxIterations: 2, Check: func(context.Context, string, int) agentio.GoalLoopOutcome {
					t.Fatal("budget exhaustion ran completion validator")
					return agentio.GoalLoopOutcome{}
				}})
				terminals := 0
				result, err := service.Execute(context.Background(), "continue", nil, func(e Event) {
					if e.Kind == "terminal" {
						terminals++
						if e.Status != agentio.BudgetExhausted || e.Reason != tc.reason || e.Verified {
							t.Fatalf("terminal %+v", e)
						}
					}
				})
				if err != nil || result.Status != agentio.BudgetExhausted || result.ExitCode() != 4 || terminals != 1 {
					t.Fatalf("result %+v err %v terminals %d", result, err, terminals)
				}
			})
		}
	}
}
