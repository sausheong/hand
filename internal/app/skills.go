package app

import (
	"context"
	"errors"
)

// ReloadSkills refreshes the model's skill index under application ownership.
// It neither modifies stores nor changes execution permissions.
func (c *Controller) ReloadSkills(ctx context.Context) error {
	operation, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return err
	}
	defer release()
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := operation.Err(); err != nil {
		return err
	}
	if c.Rt == nil {
		return errors.New("runtime unavailable")
	}
	return c.Rt.RefreshSkills()
}
