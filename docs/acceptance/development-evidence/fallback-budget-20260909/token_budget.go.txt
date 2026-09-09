package app

import (
	"context"
	"errors"
)

// TokenBudgetView is bounded independently of the ledger's request history.
// Committed includes uncertain and outstanding reservations, not just reported
// usage. A zero limit means no session token ceiling has been configured.
type TokenBudgetView struct {
	Limit          int64  `json:"limit"`
	Committed      int64  `json:"committed"`
	Exhausted      bool   `json:"exhausted"`
	Attempts       int    `json:"attempts"`
	Uncertain      int    `json:"uncertain_attempts"`
	EstimateMethod string `json:"estimate_method"`
}

func (c *Controller) TokenBudget(ctx context.Context) (TokenBudgetView, error) {
	operation, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return TokenBudgetView{}, err
	}
	defer release()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Rt == nil {
		return TokenBudgetView{}, errors.New("runtime unavailable")
	}
	s, err := c.Rt.TokenBudget(operation)
	if err != nil {
		return TokenBudgetView{}, err
	}
	view := TokenBudgetView{Limit: s.Limit, Committed: s.Committed, Exhausted: s.Exhausted, Attempts: len(s.Attempts), EstimateMethod: "UTF-8 bytes/4 plus framing and fixed image allowance; output cap reserved; not an invoice guarantee"}
	for _, a := range s.Attempts {
		if a.Status == "reserved" || a.Status == "unknown" {
			view.Uncertain++
		}
	}
	return view, nil
}
func (c *Controller) DecideTokenBudget(ctx context.Context, limit int64) error {
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
	return c.Rt.DecideTokenBudget(operation, limit)
}
