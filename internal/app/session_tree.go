package app

import (
	"context"
	"errors"

	"github.com/sausheong/harness/session"
)

// SessionNode exposes conversation topology without copying payloads into the UI.
type SessionNode struct {
	ID, ParentID, Type, Role string
	Selected                 bool
}

func (c *Controller) SessionTree(ctx context.Context) ([]SessionNode, error) {
	_, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return nil, err
	}
	defer release()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.Rt == nil || c.Rt.Session == nil {
		return nil, errors.New("session unavailable")
	}
	if c.Rt.Compaction.HasInFlight(c.Rt.Session) {
		return nil, errors.New("background compaction is still active")
	}
	leaf := c.Rt.Session.LeafID()
	nodes := make([]SessionNode, 0)
	for _, entry := range c.Rt.Session.Entries() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.Type == session.EntryTypeSelection || entry.Type == session.EntryTypeHeader || entry.Type == session.EntryTypeAnnotation {
			continue
		}
		nodes = append(nodes, SessionNode{ID: entry.ID, ParentID: entry.ParentID, Type: string(entry.Type), Role: entry.Role, Selected: entry.ID == leaf})
	}
	return nodes, nil
}

// SelectSessionNode persists the selected leaf before reporting success. Future
// messages extend that node; all other branches remain in the session graph.
func (c *Controller) SelectSessionNode(ctx context.Context, id string) error {
	_, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return err
	}
	defer release()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Rt == nil || c.Rt.Session == nil {
		return errors.New("session unavailable")
	}
	if c.Rt.Compaction.HasInFlight(c.Rt.Session) {
		return errors.New("background compaction is still active")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.Rt.Session.Branch(id); err != nil {
		return err
	}
	c.sessionWarning = ""
	return nil
}
