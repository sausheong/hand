package app

import (
	"context"
	"errors"
)

func (c *Controller) ForkSession(ctx context.Context) error {
	_, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return err
	}
	defer release()
	previousSession := c.SessionID()
	defer c.observeSessionSelection(ctx, previousSession)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Sessions == nil || c.Rt == nil || c.Rt.Session == nil {
		return errors.New("workspace sessions unavailable")
	}
	old := c.Rt.Session
	if c.Rt.Compaction.HasInFlight(old) {
		return errors.New("background compaction is still active")
	}
	if err := old.Flush(); err != nil {
		return err
	}
	selected, err := c.Sessions.Fork(ctx, old.History())
	if err != nil {
		return err
	}
	c.Rt.Session, c.SessionKey = selected.Session, selected.Record.StoreKey
	c.sessionWarning = ""
	c.owner().mu.Lock()
	c.owner().options.SessionID = selected.Session.ID
	c.owner().mu.Unlock()
	if err := old.Close(); err != nil {
		c.sessionWarning = "fork selected; previous writer cleanup failed: " + err.Error()
	}
	return nil
}
