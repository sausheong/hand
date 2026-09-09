package app

import (
	"errors"
	"github.com/sausheong/hand/internal/sessionio"
)

func (c *Controller) SessionUsage() (sessionio.UsageSummary, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Rt == nil {
		return sessionio.UsageSummary{}, errors.New("runtime unavailable")
	}
	return sessionio.ReadUsage(c.Rt.Session)
}
