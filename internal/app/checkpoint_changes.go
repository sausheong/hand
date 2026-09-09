package app

import (
	"context"
	"errors"

	"github.com/sausheong/hand/internal/checkpoints"
)

type CheckpointChangesPage struct {
	RunID           string               `json:"run_id"`
	Before          string               `json:"before"`
	After           string               `json:"after"`
	Changes         []checkpoints.Change `json:"changes"`
	Next            int                  `json:"next"`
	Total           int                  `json:"total"`
	BeforeOmissions int                  `json:"before_omissions"`
	AfterOmissions  int                  `json:"after_omissions"`
}

// CheckpointChanges reads durable run associations and integrity-checked
// snapshots. Empty runID selects the latest start, including an interrupted
// one; incomplete/expired evidence is an error rather than an empty diff.
func (c *Controller) CheckpointChanges(ctx context.Context, runID string, offset int) (CheckpointChangesPage, error) {
	operation, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return CheckpointChangesPage{}, err
	}
	defer release()
	c.mu.RLock()
	defer c.mu.RUnlock()
	boundary, ok := c.owner().options.RunBoundary.(*WorkspaceCheckpoints)
	if !ok || boundary.Store == nil {
		return CheckpointChangesPage{}, errors.New("checkpoint capture is not configured")
	}
	if len(runID) > 128 || offset < 0 {
		return CheckpointChangesPage{}, errors.New("invalid checkpoint selection")
	}
	if c.Rt == nil || c.Rt.Session == nil {
		return CheckpointChangesPage{}, errors.New("checkpoint session unavailable")
	}
	records, err := ReadSessionCheckpoints(c.Rt.Session)
	if err != nil {
		return CheckpointChangesPage{}, err
	}
	var selected *SessionCheckpoint
	for i := range records {
		if runID == "" || records[i].Start.RunID == runID {
			selected = &records[i]
		}
	}
	if selected == nil {
		return CheckpointChangesPage{}, errors.New("checkpoint run not found")
	}
	if selected.Finish == nil || selected.Finish.After == "" || selected.Finish.Error != "" {
		return CheckpointChangesPage{}, errors.New("checkpoint run has no complete after-image")
	}
	before, err := boundary.Store.Load(operation, selected.Start.Before)
	if err != nil {
		return CheckpointChangesPage{}, err
	}
	after, err := boundary.Store.Load(operation, selected.Finish.After)
	if err != nil {
		return CheckpointChangesPage{}, err
	}
	changes := checkpoints.Changes(before, after)
	if offset > len(changes) {
		return CheckpointChangesPage{}, errors.New("invalid checkpoint offset")
	}
	end := min(offset+32, len(changes))
	return CheckpointChangesPage{RunID: selected.Start.RunID, Before: before.Digest(), After: after.Digest(), Changes: changes[offset:end], Next: end, Total: len(changes), BeforeOmissions: len(before.Omissions()), AfterOmissions: len(after.Omissions())}, nil
}

type CheckpointRestorePreview struct {
	RunID     string                      `json:"run_id"`
	Before    string                      `json:"before"`
	After     string                      `json:"after"`
	Current   string                      `json:"current"`
	Actions   []checkpoints.RestoreAction `json:"actions"`
	Conflicts bool                        `json:"conflicts"`
}

// PreviewCheckpointRestore never writes files or publishes a new checkpoint.
// Its current digest identifies the reviewed state, not future write authority.
func (c *Controller) PreviewCheckpointRestore(ctx context.Context, runID string, paths []string) (CheckpointRestorePreview, error) {
	operation, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return CheckpointRestorePreview{}, err
	}
	defer release()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if runID == "" || len(runID) > 128 || len(paths) == 0 || len(paths) > 32 {
		return CheckpointRestorePreview{}, errors.New("restore preview requires run ID and 1-32 selected paths")
	}
	boundary, ok := c.owner().options.RunBoundary.(*WorkspaceCheckpoints)
	if !ok || boundary.Store == nil {
		return CheckpointRestorePreview{}, errors.New("checkpoint capture is not configured")
	}
	if c.Rt == nil || c.Rt.Session == nil {
		return CheckpointRestorePreview{}, errors.New("checkpoint session unavailable")
	}
	records, err := ReadSessionCheckpoints(c.Rt.Session)
	if err != nil {
		return CheckpointRestorePreview{}, err
	}
	var selected *SessionCheckpoint
	for i := range records {
		if records[i].Start.RunID == runID {
			selected = &records[i]
			break
		}
	}
	if selected == nil || selected.Finish == nil || selected.Finish.After == "" || selected.Finish.Error != "" {
		return CheckpointRestorePreview{}, errors.New("checkpoint run has no complete after-image")
	}
	before, err := boundary.Store.Load(operation, selected.Start.Before)
	if err != nil {
		return CheckpointRestorePreview{}, err
	}
	after, err := boundary.Store.Load(operation, selected.Finish.After)
	if err != nil {
		return CheckpointRestorePreview{}, err
	}
	if boundary.Processes != nil {
		resume, err := boundary.Processes.pauseForCheckpoint(operation)
		if err != nil {
			return CheckpointRestorePreview{}, err
		}
		defer resume()
	}
	current, err := checkpoints.Capture(operation, boundary.Workspace, boundary.Limits)
	if err != nil {
		return CheckpointRestorePreview{}, err
	}
	plan, err := checkpoints.PlanRestore(before, after, current, paths)
	if err != nil {
		return CheckpointRestorePreview{}, err
	}
	return CheckpointRestorePreview{RunID: runID, Before: before.Digest(), After: after.Digest(), Current: current.Digest(), Actions: plan.Actions(), Conflicts: plan.HasConflicts()}, nil
}
