package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

const selectedRunBudgetKind = "hand.budget.run.v1"

type runBudgetSelection struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
}

// Selection is explicit and durable. New prompts and process restarts do not
// silently grant fresh allowances; selecting another configured run is explicit.
func (c *Controller) SelectRunBudget(ctx context.Context, id string) error {
	return c.withCostBudget(ctx, func(ctx context.Context, r *runtime.Runtime) error {
		state, err := r.RunBudget(ctx, id)
		if err != nil {
			return err
		}
		if state.Tokens.Limit == 0 && state.Cost.LimitNano == 0 && state.Deadline.Version == 0 {
			return errors.New("run budget has no explicit decision")
		}
		raw, err := json.Marshal(runBudgetSelection{Version: 1, ID: id})
		if err != nil {
			return err
		}
		return r.Session.Annotate(selectedRunBudgetKind, raw)
	})
}
func (c *Controller) RunBudget(ctx context.Context, id string) (state runtime.RunBudgetState, err error) {
	err = c.withCostBudget(ctx, func(ctx context.Context, r *runtime.Runtime) error {
		var e error
		state, e = r.RunBudget(ctx, id)
		return e
	})
	return
}
func (c *Controller) DecideRunTokenBudget(ctx context.Context, id string, limit int64) error {
	return c.withCostBudget(ctx, func(ctx context.Context, r *runtime.Runtime) error { return r.DecideRunTokenBudget(ctx, id, limit) })
}
func (c *Controller) DecideRunCostBudget(ctx context.Context, id, currency string, limit int64, strict bool) error {
	return c.withCostBudget(ctx, func(ctx context.Context, r *runtime.Runtime) error {
		return r.DecideRunCostBudget(ctx, id, currency, limit, strict)
	})
}
func (c *Controller) DecideRunTimeBudget(ctx context.Context, id string, seconds int64) error {
	if seconds < 1 || seconds > 2592000 {
		return errors.New("time allowance must be 1-2592000 seconds")
	}
	return c.withCostBudget(ctx, func(ctx context.Context, r *runtime.Runtime) error {
		return r.DecideRunTimeBudget(ctx, id, time.Duration(seconds)*time.Second)
	})
}

type runBudgetPreparer interface {
	PrepareSelectedRunBudget(context.Context) (context.Context, context.CancelFunc, string, error)
}

func (b *HarnessBackend) PrepareSelectedRunBudget(ctx context.Context) (context.Context, context.CancelFunc, string, error) {
	noop := func() {}
	if b.Runtime == nil || b.Runtime.Session == nil {
		return ctx, noop, "", errors.New("runtime session unavailable")
	}
	id, err := readSelectedRunBudget(b.Runtime.Session)
	if err != nil || id == "" {
		return ctx, noop, "", err
	}
	bound, cancel, err := b.Runtime.PrepareRunBudget(ctx, id)
	return bound, cancel, id, err
}

func readSelectedRunBudget(sess *session.Session) (string, error) {
	if sess == nil {
		return "", errors.New("session unavailable")
	}
	records := sess.Annotations(selectedRunBudgetKind)
	if len(records) == 0 {
		return "", nil
	}
	var selection runBudgetSelection
	d := json.NewDecoder(bytes.NewReader(records[len(records)-1].Payload))
	opening, err := d.Token()
	if err != nil || opening != json.Delim('{') {
		return "", errors.New("invalid selected run budget object")
	}
	fields := make(map[string]json.RawMessage, 2)
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return "", err
		}
		key, ok := token.(string)
		if !ok || (key != "version" && key != "id") || fields[key] != nil {
			return "", errors.New("unknown or duplicate selected run budget field")
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return "", err
		}
		fields[key] = value
	}
	if closing, err := d.Token(); err != nil || closing != json.Delim('}') || len(fields) != 2 {
		return "", errors.New("incomplete selected run budget object")
	}
	if err := json.Unmarshal(fields["version"], &selection.Version); err != nil {
		return "", err
	}
	if err := json.Unmarshal(fields["id"], &selection.ID); err != nil {
		return "", err
	}
	if d.Decode(new(any)) != io.EOF || selection.Version != 1 || selection.ID == "" || len(selection.ID) > 64 || strings.IndexFunc(selection.ID, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-')
	}) >= 0 {
		return "", errors.New("invalid selected run budget")
	}
	return selection.ID, nil
}

type RunBudgetView struct {
	ID         string          `json:"id"`
	Selected   bool            `json:"selected"`
	Configured bool            `json:"configured"`
	Tokens     TokenBudgetView `json:"tokens"`
	Cost       CostBudgetView  `json:"cost"`
	Time       TimeBudgetView  `json:"time"`
	Disclosure string          `json:"disclosure"`
}

// InspectRunBudget returns counters rather than unbounded per-attempt records.
// An empty ID inspects the current selection without changing it.
func (c *Controller) InspectRunBudget(ctx context.Context, id string) (view RunBudgetView, err error) {
	err = c.withCostBudget(ctx, func(ctx context.Context, r *runtime.Runtime) error {
		selected, e := readSelectedRunBudget(r.Session)
		if e != nil {
			return e
		}
		view.Disclosure = "Selection persists across prompts and restart. Run and session limits both apply. Decisions change absolute ceilings without erasing charges."
		if id == "" {
			id = selected
		}
		if id == "" {
			return nil
		}
		state, e := r.RunBudget(ctx, id)
		if e != nil {
			return e
		}
		view.ID = id
		view.Selected = id == selected
		view.Configured = state.Tokens.Limit > 0 || state.Cost.LimitNano > 0 || state.Deadline.Version != 0
		view.Tokens = TokenBudgetView{Limit: state.Tokens.Limit, Committed: state.Tokens.Committed, Exhausted: state.Tokens.Exhausted, Attempts: len(state.Tokens.Attempts), EstimateMethod: "UTF-8 bytes/4 plus framing and fixed image allowance; output cap reserved"}
		for _, a := range state.Tokens.Attempts {
			if a.Status == "reserved" || a.Status == "unknown" {
				view.Tokens.Uncertain++
			}
		}
		view.Cost = CostBudgetView{Currency: state.Cost.Currency, LimitNano: state.Cost.LimitNano, CommittedNano: state.Cost.CommittedNano, Strict: state.Cost.Strict, Exhausted: state.Cost.Exhausted, Attempts: len(state.Cost.Attempts), Unpriced: state.Cost.Unknown, Disclosure: "Billionths of the named currency, including reservations. Unknown charges are not free; estimates and billing lag prevent an exact invoice cap."}
		for _, a := range state.Cost.Attempts {
			if a.Status == "reserved" || a.Status == "uncertain" {
				view.Cost.Uncertain++
			}
			if a.Category == llm.CallCompaction {
				view.Cost.CompactionNano += a.ChargedNano
			}
		}
		view.Time = TimeBudgetView{Configured: state.Deadline.Version != 0, DecidedAt: state.Deadline.DecidedAt, Deadline: state.Deadline.Deadline, Disclosure: "Wall-clock allowance includes idle time and completion validation. Restart does not renew it."}
		if view.Time.Configured {
			remaining := time.Until(state.Deadline.Deadline)
			view.Time.Expired = remaining <= 0
			view.Time.RemainingMillis = max(0, remaining.Milliseconds())
		}
		return nil
	})
	return
}
