package app

import (
	"context"
	"errors"
	"github.com/sausheong/harness/runtime"
)

func (c *Controller) ContextState(ctx context.Context) (runtime.ContextState, error) {
	operation, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return runtime.ContextState{}, err
	}
	defer release()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Rt == nil {
		return runtime.ContextState{}, errors.New("runtime unavailable")
	}
	return c.Rt.ContextState(operation)
}
func (c *Controller) SetContextState(ctx context.Context, revision int, items []runtime.ContextStateItem) error {
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
	return c.Rt.SetContextState(operation, revision, items)
}

// UpdateContextItem changes one item without replacing unrelated task facts.
// The revision guard also detects writes by other runtime owners of the session.
func (c *Controller) UpdateContextItem(ctx context.Context, item runtime.ContextStateItem, remove bool) error {
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
	state, err := c.Rt.ContextState(operation)
	if err != nil {
		return err
	}
	found := false
	for i := range state.Items {
		if state.Items[i].ID != item.ID {
			continue
		}
		found = true
		if remove {
			state.Items = append(state.Items[:i], state.Items[i+1:]...)
		} else {
			state.Items[i] = item
		}
		break
	}
	if !found {
		if remove {
			return errors.New("structured context item not found")
		}
		state.Items = append(state.Items, item)
	}
	return c.Rt.SetContextState(operation, state.Revision, state.Items)
}
