package rpc

import (
	"context"
	"github.com/sausheong/hand/protocol"
)

func (d *Dispatcher) dispatchRunBudget(ctx context.Context, r protocol.Request, fail func(string, string) protocol.Response, result func(any) protocol.Response) protocol.Response {
	if d.controller == nil {
		return fail("unavailable", "budget controller unavailable")
	}
	switch r.Method {
	case "budget.run":
		var p struct {
			ID string `json:"id"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		view, err := d.controller.InspectRunBudget(ctx, p.ID)
		if err != nil {
			return fail("budget_unavailable", err.Error())
		}
		return result(view)
	case "budget.run.select":
		var p struct {
			ID        string `json:"id"`
			Confirmed bool   `json:"confirmed"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		if !p.Confirmed {
			return fail("confirmation_required", "explicitly select a configured logical run; existing charges remain")
		}
		if err := d.controller.SelectRunBudget(ctx, p.ID); err != nil {
			return fail("budget_rejected", err.Error())
		}
		return result(map[string]any{"id": p.ID, "selected": true})
	case "budget.run.tokens.decide":
		var p struct {
			ID        string `json:"id"`
			Limit     int64  `json:"limit"`
			Confirmed bool   `json:"confirmed"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		if !p.Confirmed {
			return fail("confirmation_required", "explicitly confirm an absolute run token ceiling")
		}
		if err := d.controller.DecideRunTokenBudget(ctx, p.ID, p.Limit); err != nil {
			return fail("budget_rejected", err.Error())
		}
		return result(map[string]any{"id": p.ID, "limit": p.Limit})
	case "budget.run.cost.decide":
		var p struct {
			ID        string `json:"id"`
			Currency  string `json:"currency"`
			Limit     int64  `json:"limit_nano"`
			Strict    *bool  `json:"strict"`
			Confirmed bool   `json:"confirmed"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		if !p.Confirmed {
			return fail("confirmation_required", "explicitly confirm an absolute run monetary ceiling")
		}
		if p.Strict == nil {
			return fail("invalid_params", "strict or advisory mode must be explicit")
		}
		if err := d.controller.DecideRunCostBudget(ctx, p.ID, p.Currency, p.Limit, *p.Strict); err != nil {
			return fail("budget_rejected", err.Error())
		}
		return result(map[string]any{"id": p.ID, "currency": p.Currency, "limit_nano": p.Limit, "strict": *p.Strict})
	case "budget.run.time.decide":
		var p struct {
			ID        string `json:"id"`
			Seconds   int64  `json:"seconds"`
			Confirmed bool   `json:"confirmed"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		if !p.Confirmed {
			return fail("confirmation_required", "explicitly confirm a new run wall-clock allowance including idle time")
		}
		if err := d.controller.DecideRunTimeBudget(ctx, p.ID, p.Seconds); err != nil {
			return fail("budget_rejected", err.Error())
		}
		return result(map[string]any{"id": p.ID, "seconds": p.Seconds})
	}
	return fail("unknown_method", "unknown run budget method")
}
