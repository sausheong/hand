package app

import (
	"context"
	"errors"
	"time"

	"github.com/sausheong/harness/runtime"
)

type TimeBudgetView struct {
	Configured      bool      `json:"configured"`
	DecidedAt       time.Time `json:"decided_at"`
	Deadline        time.Time `json:"deadline"`
	Expired         bool      `json:"expired"`
	RemainingMillis int64     `json:"remaining_millis"`
	Disclosure      string    `json:"disclosure"`
}

func (c *Controller) TimeBudget(ctx context.Context) (view TimeBudgetView, err error) {
	err = c.withCostBudget(ctx, func(ctx context.Context, r *runtime.Runtime) error {
		state, err := r.TimeBudget(ctx)
		if err != nil {
			return err
		}
		view = TimeBudgetView{Configured: state.Version != 0, DecidedAt: state.DecidedAt, Deadline: state.Deadline, Disclosure: "Wall-clock allowance includes idle time. Restart does not renew it. A new explicit decision is required after expiry; token and monetary charges remain."}
		if view.Configured {
			remaining := time.Until(state.Deadline)
			view.Expired = remaining <= 0
			view.RemainingMillis = max(0, remaining.Milliseconds())
		}
		return nil
	})
	return
}
func (c *Controller) DecideTimeBudget(ctx context.Context, seconds int64) error {
	if seconds <= 0 || seconds > 30*24*60*60 {
		return errors.New("time allowance must be 1-2592000 seconds")
	}
	return c.withCostBudget(ctx, func(ctx context.Context, r *runtime.Runtime) error {
		return r.DecideTimeBudget(ctx, time.Duration(seconds)*time.Second)
	})
}
