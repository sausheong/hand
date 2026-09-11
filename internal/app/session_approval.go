package app

import (
	"context"
	"encoding/json"
	"sync/atomic"

	"github.com/sausheong/harness/runtime"
)

// SessionApproval is process-local user consent, separate from saved grants.
type SessionApproval struct{ skip atomic.Bool }

func (p *SessionApproval) SetSkip(skip bool) { p.skip.Store(skip) }
func (p *SessionApproval) Skipping() bool    { return p != nil && p.skip.Load() }
func (p *SessionApproval) Wrap(next func(context.Context, string, json.RawMessage) (runtime.HookDecision, error)) func(context.Context, string, json.RawMessage) (runtime.HookDecision, error) {
	return func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
		if err := ctx.Err(); err != nil {
			return runtime.HookDecision{}, err
		}
		if p.Skipping() {
			return runtime.HookDecision{Allow: true}, nil
		}
		return next(ctx, name, input)
	}
}
