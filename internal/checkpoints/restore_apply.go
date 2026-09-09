package checkpoints

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
)

type AppliedRestore struct {
	Path         string `json:"path"`
	Applied      bool   `json:"applied"`
	RecoveryPath string `json:"recovery_path,omitempty"`
}

// ApplyRestore executes an explicitly reviewed plan under store ownership.
// The caller must also hold application workspace mutation ownership throughout.
// All selected actions are validated before the first mutation. This is a series
// of durable single-file transactions, not an atomic multi-file transaction:
// on error results include every attempted file, including uncertain application.
// Retained recovery files must not be deleted until explicitly reconciled.
func (s *Store) ApplyRestore(ctx context.Context, plan *RestorePlan, limits Limits) ([]AppliedRestore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ready(); err != nil {
		return nil, err
	}
	if plan == nil || len(plan.actions) == 0 || len(plan.actions) > 32 || plan.HasConflicts() {
		return nil, errors.New("restore requires a conflict-free reviewed selection of 1-32 files")
	}
	before, err := s.load(ctx, plan.before)
	if err != nil {
		return nil, err
	}
	after, err := s.load(ctx, plan.after)
	if err != nil {
		return nil, err
	}
	events, err := s.readRestoreJournal(ctx)
	if err != nil {
		return nil, err
	}
	latest := map[string]string{}
	for _, e := range events {
		latest[e.RecoveryName] = e.Phase
	}
	for _, phase := range latest {
		if phase != "applied" && phase != "cancelled" {
			return nil, errors.New("restore recovery must be reconciled before another restore")
		}
	}
	if len(events)+2*len(plan.actions) > restoreJournalRecords {
		return nil, errors.New("insufficient restore journal capacity")
	}
	current, err := Capture(ctx, s.workspace, limits)
	if err != nil {
		return nil, err
	}
	if current.Digest() != plan.current {
		return nil, errors.New("workspace changed since restore review")
	}
	selection := make([]string, len(plan.actions))
	for i, a := range plan.actions {
		selection[i] = a.Path
	}
	checked, err := PlanRestore(before, after, current, selection)
	if err != nil {
		return nil, err
	}
	if checked.HasConflicts() || !reflect.DeepEqual(checked.actions, plan.actions) {
		return nil, errors.New("restore review differs from durable snapshots")
	}
	root, err := os.OpenRoot(s.workspace)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	results := make([]AppliedRestore, 0, len(plan.actions))
	for _, action := range checked.actions {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		result, err := restoreFile(ctx, root, action, before, s.recordRestore)
		results = append(results, AppliedRestore{action.Path, result.Applied, result.RecoveryPath})
		if err != nil {
			return results, fmt.Errorf("restore %s: %w", action.Path, err)
		}
	}
	return results, nil
}
