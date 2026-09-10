package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/checkpoints"
)

type boundaryFunc func(context.Context, string) (func(context.Context) error, error)

func (f boundaryFunc) Begin(c context.Context, id string) (func(context.Context) error, error) {
	return f(c, id)
}
func TestCheckpointBoundaryVetoAndTerminalOrdering(t *testing.T) {
	ran := false
	backend := &backendFixture{run: func(context.Context, string) (<-chan BackendEvent, error) { ran = true; return completedStream(), nil }}
	service := New(backend, Options{MaxIterations: 1, RunBoundary: boundaryFunc(func(context.Context, string) (func(context.Context) error, error) {
		return nil, errors.New("capture failed")
	})})
	result, err := service.Execute(context.Background(), "test", nil, nil)
	if err != nil || ran || result.Reason != "checkpoint_start_failed" {
		t.Fatal(result, err, ran)
	}
	finished := false
	service = New(backend, Options{MaxIterations: 1, RunBoundary: boundaryFunc(func(context.Context, string) (func(context.Context) error, error) {
		return func(context.Context) error { finished = true; return errors.New("after failed") }, nil
	})})
	result, err = service.Execute(context.Background(), "test", nil, func(e Event) {
		if e.Kind == "terminal" && !finished {
			t.Error("terminal preceded checkpoint finish")
		}
	})
	if err != nil || !ran || !finished || result.Status != agentio.InfrastructureError || result.Reason != "checkpoint_finish_failed" {
		t.Fatal(result, err)
	}
}
func TestWorkspaceCheckpointAroundApplicationRun(t *testing.T) {
	work := t.TempDir()
	file := filepath.Join(work, "file")
	if err := os.WriteFile(file, []byte("user dirty content"), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := checkpoints.OpenStore(filepath.Join(t.TempDir(), "store"), work, checkpoints.DefaultStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	boundary := &WorkspaceCheckpoints{Workspace: work, Limits: checkpoints.DefaultLimits(), Store: store}
	backend := &backendFixture{run: func(context.Context, string) (<-chan BackendEvent, error) {
		if err := os.WriteFile(file, []byte("agent changed"), 0600); err != nil {
			return nil, err
		}
		return completedStream(), nil
	}}
	service := New(backend, Options{MaxIterations: 1, RunBoundary: boundary})
	result, err := service.Execute(context.Background(), "change", nil, func(e Event) {
		if e.Kind == "terminal" && len(boundary.Pairs()) != 1 {
			t.Error("missing pair before terminal")
		}
	})
	if err != nil || result.Status != agentio.Completed {
		t.Fatal(result, err)
	}
	pairs := boundary.Pairs()
	if len(pairs) != 1 || pairs[0].Before == pairs[0].After || pairs[0].RunID == "" {
		t.Fatal(pairs)
	}
	before, err := store.Load(context.Background(), pairs[0].Before)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := before.Content(before.Records()[0].Hash)
	if string(data) != "user dirty content" {
		t.Fatal("lost starting dirty content")
	}
	pairs[0].Before = "mutated"
	if boundary.Pairs()[0].Before == "mutated" {
		t.Fatal("mutable pair summary")
	}
}
