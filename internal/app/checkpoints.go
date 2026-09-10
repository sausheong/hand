package app

import (
	"context"
	"errors"
	"path/filepath"
	"sync"

	"github.com/sausheong/hand/internal/checkpoints"
)

// RunBoundary participates in the owned application operation. Begin must
// finish before backend execution; its returned finish function is joined
// before terminal publication, including after cancellation.
type RunBoundary interface {
	Begin(context.Context, string) (func(context.Context) error, error)
}

type CheckpointPair struct{ RunID, Before, After, Error string }

// WorkspaceCheckpoints is configured before service use. Pair summaries are
// bounded in memory; snapshot bytes are durable in Store. The application must
// ensure background writers are idle before enabling this boundary.
type WorkspaceCheckpoints struct {
	Record    func(string, CheckpointPair) error // optional durable start/finish recorder
	Processes *Processes                         // when present, background admission is paused during each capture
	Workspace string
	Limits    checkpoints.Limits
	Store     *checkpoints.Store
	mu        sync.Mutex
	pairs     []CheckpointPair
}

func (c *WorkspaceCheckpoints) Pairs() []CheckpointPair {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]CheckpointPair(nil), c.pairs...)
}
func (c *WorkspaceCheckpoints) Begin(ctx context.Context, id string) (func(context.Context) error, error) {
	if c.Store == nil {
		return nil, errors.New("checkpoint store unavailable")
	}
	before, err := c.capture(ctx)
	if err != nil {
		return nil, err
	}
	if c.Record != nil {
		if err = c.Record("start", CheckpointPair{RunID: id, Before: before.Digest()}); err != nil {
			return nil, err
		}
	}
	return func(end context.Context) error {
		pair := CheckpointPair{RunID: id, Before: before.Digest()}
		after, err := c.capture(end)
		if err != nil {
			pair.Error = err.Error()
		} else {
			pair.After = after.Digest()
		}
		if c.Record != nil {
			if recordErr := c.Record("finish", pair); recordErr != nil {
				err = errors.Join(err, recordErr)
				pair.Error = err.Error()
			}
		}
		c.mu.Lock()
		if len(c.pairs) == 100 {
			copy(c.pairs, c.pairs[1:])
			c.pairs = c.pairs[:99]
		}
		c.pairs = append(c.pairs, pair)
		c.mu.Unlock()
		return err
	}, nil
}

func (c *WorkspaceCheckpoints) capture(ctx context.Context) (*checkpoints.Snapshot, error) {
	if c.Processes != nil {
		work, err := filepath.EvalSymlinks(c.Workspace)
		if err != nil {
			return nil, err
		}
		owner, err := filepath.EvalSymlinks(c.Processes.workspace)
		if err != nil {
			return nil, err
		}
		work, err = filepath.Abs(work)
		if err != nil {
			return nil, err
		}
		owner, err = filepath.Abs(owner)
		if err != nil {
			return nil, err
		}
		if work != owner {
			return nil, errors.New("checkpoint background owner workspace mismatch")
		}
		release, err := c.Processes.pauseForCheckpoint(ctx)
		if err != nil {
			return nil, err
		}
		defer release()
	}
	snap, err := checkpoints.Capture(ctx, c.Workspace, c.Limits)
	if err != nil {
		return nil, err
	}
	if err = c.Store.Save(ctx, snap); err != nil {
		return nil, err
	}
	return snap, nil
}

// ConfigureCheckpoints binds recording to this service's live Harness session.
// Configuration is admitted only while idle and stores remain caller-owned.
func (s *Service) ConfigureCheckpoints(c *WorkspaceCheckpoints) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active {
		return ErrBusy
	}
	if c == nil || c.Store == nil {
		return errors.New("checkpoint configuration requires store")
	}
	backend, ok := s.backend.(*HarnessBackend)
	if !ok {
		return errors.New("checkpoint session journal requires Harness backend")
	}
	c.Record = backend.RecordCheckpoint
	s.options.RunBoundary = c
	return nil
}
