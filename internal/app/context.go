package app

import (
	"context"
	"errors"
	"github.com/sausheong/harness/runtime"
)

func (c *Controller) InspectContext(ctx context.Context) (runtime.ContextInspection, error) {
	var result runtime.ContextInspection
	operation, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return result, err
	}
	defer release()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Rt == nil {
		return result, errors.New("runtime unavailable")
	}
	return c.Rt.InspectContext(operation)
}
