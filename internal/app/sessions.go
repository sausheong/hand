package app

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/harness/session"
)

func (c *Controller) ListSessions() ([]sessionio.SessionRecord, error) {
	return c.ListSessionsContext(context.Background())
}

func (c *Controller) ListSessionsContext(ctx context.Context) ([]sessionio.SessionRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Sessions == nil || c.Rt == nil {
		return nil, errors.New("workspace sessions unavailable")
	}
	snapshot, err := c.Sessions.Catalogue().Snapshot()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	records := make([]sessionio.SessionRecord, 0, len(snapshot.Sessions))
	for _, record := range snapshot.Sessions {
		if record.AgentID == c.Rt.AgentID {
			records = append(records, record)
		}
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].LastActive.Equal(records[j].LastActive) {
			return records[i].ID < records[j].ID
		}
		return records[i].LastActive.After(records[j].LastActive)
	})
	return records, nil
}

func (c *Controller) RenameSession(name string) error {
	return c.RenameSessionContext(context.Background(), name)
}

func (c *Controller) RenameSessionContext(ctx context.Context, name string) error {
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
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.Sessions.Catalogue().Rename(c.Rt.Session.ID, name)
}

// ResumeSession preserves the current writer and view until the target has
// been validated, leased and selected successfully.
func (c *Controller) ResumeSession(id string) error {
	return c.ResumeSessionContext(context.Background(), id)
}

func (c *Controller) ResumeSessionContext(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("session ID is required")
	}
	_, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return err
	}
	defer release()
	previousSession := c.SessionID()
	defer c.observeSessionSelection(ctx, previousSession)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.Sessions == nil || c.Rt == nil {
		return errors.New("workspace sessions unavailable")
	}
	if c.Rt.Compaction.HasInFlight(c.Rt.Session) {
		return errors.New("background compaction is still active")
	}
	old := c.Rt.Session
	if old != nil {
		if err := old.Flush(); err != nil {
			return fmt.Errorf("save current session: %w", err)
		}
		if old.ID == id {
			return c.Sessions.Catalogue().Select(id)
		}
	}
	selected, err := c.Sessions.Open(ctx, id, false)
	if err != nil {
		return err
	}
	c.Rt.Session = selected.Session
	c.SessionKey = selected.Record.StoreKey
	c.sessionWarning = ""
	c.owner().mu.Lock()
	c.owner().options.SessionID = selected.Session.ID
	c.owner().mu.Unlock()
	if old != nil {
		if err := old.Close(); err != nil {
			c.sessionWarning = "session resumed; previous writer cleanup failed: " + err.Error()
		}
	}
	if len(selected.Backups) > 0 {
		c.sessionWarning += fmt.Sprintf(" session recovery preserved %d backup(s)", len(selected.Backups))
	}
	return nil
}

func (c *Controller) SessionHistory() []session.SessionEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Rt == nil || c.Rt.Session == nil {
		return nil
	}
	return c.Rt.Session.History()
}
