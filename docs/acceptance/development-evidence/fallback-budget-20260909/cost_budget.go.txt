package app

import (
	"context"
	"errors"

	"github.com/sausheong/harness/budget"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
)

type CostBudgetView struct {
	Currency       string `json:"currency"`
	LimitNano      int64  `json:"limit_nano"`
	CommittedNano  int64  `json:"committed_nano"`
	CompactionNano int64  `json:"compaction_nano"`
	Strict         bool   `json:"strict"`
	Exhausted      bool   `json:"exhausted"`
	Attempts       int    `json:"attempts"`
	Unpriced       int    `json:"unpriced_attempts"`
	Uncertain      int    `json:"uncertain_attempts"`
	Disclosure     string `json:"disclosure"`
}

func (c *Controller) withCostBudget(ctx context.Context, action func(context.Context, *runtime.Runtime) error) error {
	operation, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return err
	}
	defer release()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Rt == nil {
		return errors.New("runtime unavailable")
	}
	return action(operation, c.Rt)
}
func (c *Controller) CostBudget(ctx context.Context) (view CostBudgetView, err error) {
	err = c.withCostBudget(ctx, func(ctx context.Context, r *runtime.Runtime) error {
		state, err := r.CostBudget(ctx)
		if err != nil {
			return err
		}
		view = CostBudgetView{Currency: state.Currency, LimitNano: state.LimitNano, CommittedNano: state.CommittedNano, Strict: state.Strict, Exhausted: state.Exhausted, Attempts: len(state.Attempts), Unpriced: state.Unknown, Disclosure: "Amounts are billionths of the named currency and include reservations. Unknown charges are not free. Estimates, billing lag and non-cancellable requests prevent an exact invoice cap."}
		for _, a := range state.Attempts {
			if a.Status == "reserved" || a.Status == "uncertain" {
				view.Uncertain++
			}
			if a.Category == llm.CallCompaction {
				view.CompactionNano += a.ChargedNano
			}
		}
		return nil
	})
	return
}
func (c *Controller) DecideCostBudget(ctx context.Context, currency string, limitNano int64, strict bool) error {
	return c.withCostBudget(ctx, func(ctx context.Context, r *runtime.Runtime) error {
		return r.DecideCostBudget(ctx, currency, limitNano, strict)
	})
}
func (c *Controller) CostPrices(ctx context.Context) (prices []budget.PriceSnapshot, err error) {
	err = c.withCostBudget(ctx, func(ctx context.Context, r *runtime.Runtime) error {
		var err error
		prices, err = r.CostPrices(ctx)
		return err
	})
	return
}
func (c *Controller) SetCostPrices(ctx context.Context, prices []budget.PriceSnapshot) error {
	return c.withCostBudget(ctx, func(ctx context.Context, r *runtime.Runtime) error { return r.SetCostPrices(ctx, prices) })
}
