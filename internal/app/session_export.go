package app

import (
	"context"
	"errors"
)

func (c *Controller) ExportSession(ctx context.Context, destination string) error {
	_, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return err
	}
	defer release()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Sessions == nil || c.Rt == nil || c.Rt.Session == nil {
		return errors.New("workspace sessions unavailable")
	}
	if c.Rt.Compaction.HasInFlight(c.Rt.Session) {
		return errors.New("background compaction is still active")
	}
	return c.Sessions.Export(ctx, c.Rt.Session, c.SessionKey, destination)
}
