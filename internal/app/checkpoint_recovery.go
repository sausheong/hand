package app

import (
	"context"
	"errors"

	"github.com/sausheong/hand/internal/checkpoints"
)

type CheckpointRecoveryPage struct {
	Recoveries []checkpoints.RestoreRecovery `json:"recoveries"`
	Next       int                           `json:"next"`
	Total      int                           `json:"total"`
}

// Recovery belongs to the configured workspace store, across sessions. It must
// remain available even when the session that initiated a restore is lost.
func (c *Controller) withCheckpointRecovery(ctx context.Context, fn func(context.Context, *checkpoints.Store) error) error {
	operation, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return err
	}
	defer release()
	c.mu.RLock()
	defer c.mu.RUnlock()
	boundary, ok := c.owner().options.RunBoundary.(*WorkspaceCheckpoints)
	if !ok || boundary.Store == nil {
		return errors.New("checkpoint capture is not configured")
	}
	if boundary.Processes != nil {
		resume, err := boundary.Processes.pauseForCheckpoint(operation)
		if err != nil {
			return err
		}
		defer resume()
	}
	return fn(operation, boundary.Store)
}

func (c *Controller) CheckpointRecoveries(ctx context.Context, offset int) (page CheckpointRecoveryPage, err error) {
	if offset < 0 {
		return page, errors.New("invalid recovery offset")
	}
	err = c.withCheckpointRecovery(ctx, func(ctx context.Context, store *checkpoints.Store) error {
		states, err := store.InspectRestores(ctx)
		if err != nil {
			return err
		}
		if offset > len(states) {
			return errors.New("invalid recovery offset")
		}
		end := min(offset+32, len(states))
		page = CheckpointRecoveryPage{Recoveries: states[offset:end], Next: end, Total: len(states)}
		return nil
	})
	return page, err
}

// ResolveCheckpointRecovery accepts an explicitly confirmed inspected record.
// "acknowledge" records an already-applied restore; "cancel" abandons an
// unchanged preparation. Neither action deletes retained recovery files.
func (c *Controller) ResolveCheckpointRecovery(ctx context.Context, action string, reviewed checkpoints.RestoreRecovery) error {
	if action != "acknowledge" && action != "cancel" {
		return errors.New("recovery action must be acknowledge or cancel")
	}
	return c.withCheckpointRecovery(ctx, func(ctx context.Context, store *checkpoints.Store) error {
		if action == "acknowledge" {
			return store.ReconcileAppliedRestore(ctx, reviewed)
		}
		return store.CancelPreparedRestore(ctx, reviewed)
	})
}
