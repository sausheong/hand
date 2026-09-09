package app

import (
	"context"
	"errors"
	"reflect"

	"github.com/sausheong/hand/internal/checkpoints"
)

// ApplyCheckpointRestore requires the exact preview explicitly confirmed by the
// caller. It owns the application and pauses background admission through final
// journal publication. Errors may accompany partial single-file results; callers
// must display those results and must not automatically retry a failed restore.
func (c *Controller) ApplyCheckpointRestore(ctx context.Context, reviewed CheckpointRestorePreview) ([]checkpoints.AppliedRestore, error) {
	operation, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return nil, err
	}
	defer release()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if reviewed.RunID == "" || len(reviewed.RunID) > 128 || reviewed.Conflicts || len(reviewed.Actions) == 0 || len(reviewed.Actions) > 32 {
		return nil, errors.New("restore requires an explicitly confirmed conflict-free preview")
	}
	boundary, ok := c.owner().options.RunBoundary.(*WorkspaceCheckpoints)
	if !ok || boundary.Store == nil {
		return nil, errors.New("checkpoint capture is not configured")
	}
	if c.Rt == nil || c.Rt.Session == nil {
		return nil, errors.New("checkpoint session unavailable")
	}
	records, err := ReadSessionCheckpoints(c.Rt.Session)
	if err != nil {
		return nil, err
	}
	var selected *SessionCheckpoint
	for i := range records {
		if records[i].Start.RunID == reviewed.RunID {
			selected = &records[i]
			break
		}
	}
	if selected == nil || selected.Finish == nil || selected.Finish.Error != "" || selected.Start.Before != reviewed.Before || selected.Finish.After != reviewed.After {
		return nil, errors.New("restore review no longer matches this session's checkpoint run")
	}
	before, err := boundary.Store.Load(operation, reviewed.Before)
	if err != nil {
		return nil, err
	}
	after, err := boundary.Store.Load(operation, reviewed.After)
	if err != nil {
		return nil, err
	}
	if boundary.Processes != nil {
		resume, err := boundary.Processes.pauseForCheckpoint(operation)
		if err != nil {
			return nil, err
		}
		defer resume()
	}
	current, err := checkpoints.Capture(operation, boundary.Workspace, boundary.Limits)
	if err != nil {
		return nil, err
	}
	if current.Digest() != reviewed.Current {
		return nil, errors.New("workspace changed since restore confirmation")
	}
	paths := make([]string, len(reviewed.Actions))
	for i, a := range reviewed.Actions {
		paths[i] = a.Path
	}
	plan, err := checkpoints.PlanRestore(before, after, current, paths)
	if err != nil {
		return nil, err
	}
	if plan.HasConflicts() || !reflect.DeepEqual(plan.Actions(), reviewed.Actions) {
		return nil, errors.New("confirmed restore actions differ from current checkpoint evidence")
	}
	return boundary.Store.ApplyRestore(operation, plan, boundary.Limits)
}
