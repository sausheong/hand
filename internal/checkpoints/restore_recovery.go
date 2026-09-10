package checkpoints

import (
	"context"
	"errors"
	"os"
	"path"
	"reflect"
	"strings"
)

type RestoreRecovery struct {
	Event  RestoreEvent `json:"event"`
	State  string       `json:"state"`
	Reason string       `json:"reason,omitempty"`
}

// InspectRestores reconciles observations without changing workspace or journal.
// Callers must keep Hand mutations quiescent; external edits can make the result
// stale. Missing or contradictory bytes are conflicts, never implicit success.
func (s *Store) InspectRestores(ctx context.Context) ([]RestoreRecovery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inspectRestores(ctx)
}

// inspectRestores requires s.mu.
func (s *Store) inspectRestores(ctx context.Context) ([]RestoreRecovery, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	events, err := s.readRestoreJournal(ctx)
	if err != nil {
		return nil, err
	}
	latest := map[string]RestoreEvent{}
	var order []string
	for _, e := range events {
		if _, ok := latest[e.RecoveryName]; !ok {
			order = append(order, e.RecoveryName)
		}
		latest[e.RecoveryName] = e
	}
	root, err := os.OpenRoot(s.workspace)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	result := make([]RestoreRecovery, 0, len(order))
	for _, id := range order {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		e := latest[id]
		r := RestoreRecovery{Event: e, State: "conflict"}
		parent, err := restoreParent(root, path.Dir(e.Path))
		if err != nil {
			r.Reason = err.Error()
			result = append(result, r)
			continue
		}
		target := path.Base(e.Path)
		expected := matchRestoreFile(ctx, parent, target, e.Expected) == nil
		restored := matchRestoreFile(ctx, parent, target, e.Restored) == nil
		backupExpected := matchRestoreFile(ctx, parent, e.RecoveryName, e.Expected) == nil
		backupRestored := matchRestoreFile(ctx, parent, e.RecoveryName, e.Restored) == nil
		absent := matchRestoreFile(ctx, parent, e.RecoveryName, nil) == nil
		parent.Close()
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		prepared, applied := false, false
		switch e.Operation {
		case "create":
			prepared = expected && backupRestored
			applied = restored && absent
		case "replace":
			prepared = expected && backupRestored
			applied = restored && backupExpected
		case "remove":
			prepared = expected && absent
			applied = restored && backupExpected
		}
		switch {
		case prepared && applied:
			r.Reason = "ambiguous restore images"
		case applied && e.Phase != "cancelled":
			if e.Phase == "applied" {
				r.State = "applied"
			} else {
				r.State = "applied_unrecorded"
			}
		case prepared && (e.Phase == "prepared" || e.Phase == "cancelled"):
			r.State = e.Phase
		default:
			r.Reason = "workspace or recovery content differs from journal"
		}
		result = append(result, r)
	}
	return result, nil
}

// Parent symlinks are explicitly rejected; a restore journal may not redirect
// recovery inspection to a different workspace subtree through an alias.
func restoreParent(root *os.Root, name string) (*os.Root, error) {
	if name != "." {
		prefix := ""
		for _, part := range strings.Split(name, "/") {
			prefix = path.Join(prefix, part)
			info, err := root.Lstat(prefix)
			if err != nil {
				return nil, err
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return nil, errors.New("restore parent is not a direct directory")
			}
		}
	}
	return root.OpenRoot(name)
}

// ReconcileAppliedRestore durably acknowledges an already-applied transaction.
// The caller must explicitly select an inspected recovery and hold application
// workspace ownership. It never repeats the filesystem mutation or deletes
// recovery bytes. Prepared/conflicting states require separate resolution.
func (s *Store) ReconcileAppliedRestore(ctx context.Context, reviewed RestoreRecovery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if reviewed.State != "applied_unrecorded" && reviewed.State != "applied" {
		return errors.New("recovery is not an applied transaction")
	}
	if err := validateRestoreEvent(reviewed.Event); err != nil {
		return err
	}
	states, err := s.inspectRestores(ctx)
	if err != nil {
		return err
	}
	for _, current := range states {
		if current.Event.RecoveryName != reviewed.Event.RecoveryName {
			continue
		}
		a, b := current.Event, reviewed.Event
		a.Phase = ""
		b.Phase = ""
		if !reflect.DeepEqual(a, b) {
			return errors.New("reviewed recovery intent differs from journal")
		}
		if current.State == "applied" {
			return nil
		}
		if current.State != "applied_unrecorded" {
			return errors.New("recovery files changed since review")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		event := current.Event
		event.Phase = "applied"
		return s.recordRestore(event)
	}
	return errors.New("restore recovery identity not found")
}

// CancelPreparedRestore records explicit abandonment of an unchanged prepared
// transaction. Target and staging files are preserved, including on retry.
// Callers hold application workspace ownership; external edits remain possible.
func (s *Store) CancelPreparedRestore(ctx context.Context, reviewed RestoreRecovery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if reviewed.State != "prepared" && reviewed.State != "cancelled" {
		return errors.New("only a prepared restore can be cancelled")
	}
	if err := validateRestoreEvent(reviewed.Event); err != nil {
		return err
	}
	states, err := s.inspectRestores(ctx)
	if err != nil {
		return err
	}
	for _, current := range states {
		if current.Event.RecoveryName != reviewed.Event.RecoveryName {
			continue
		}
		a, b := current.Event, reviewed.Event
		a.Phase = ""
		b.Phase = ""
		if !reflect.DeepEqual(a, b) {
			return errors.New("reviewed recovery intent differs from journal")
		}
		if current.State == "cancelled" {
			return nil
		}
		if current.State != "prepared" {
			return errors.New("restore is no longer an unchanged preparation")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		event := current.Event
		event.Phase = "cancelled"
		return s.recordRestore(event)
	}
	return errors.New("restore recovery identity not found")
}
