//go:build unix

package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/sausheong/hand/internal/checkpoints"
)

func TestCheckpointPausesBackgroundAdmission(t *testing.T) {
	p := testProcesses(t)
	release, err := p.pauseForCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Start(context.Background(), "echo forbidden"); err == nil {
		t.Fatal("process started during capture")
	}
	if _, err = p.pauseForCheckpoint(context.Background()); err == nil {
		t.Fatal("nested capture accepted")
	}
	release()
	release()
	info, err := p.Start(context.Background(), "echo resumed")
	if err != nil {
		t.Fatal(err)
	}
	waitProcess(t, p, info.ID)
	release, err = p.pauseForCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release()
}
func TestCheckpointRejectsRunningBackground(t *testing.T) {
	p := testProcesses(t)
	info, err := p.Start(context.Background(), "read input")
	if err != nil {
		t.Fatal(err)
	}
	store, err := checkpoints.OpenStore(filepath.Join(t.TempDir(), "store"), p.workspace, checkpoints.DefaultStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	boundary := &WorkspaceCheckpoints{Workspace: p.workspace, Limits: checkpoints.DefaultLimits(), Store: store, Processes: p}
	if _, err = boundary.Begin(context.Background(), "running"); err == nil {
		t.Fatal("captured live background writer")
	}
	if err = p.Cancel(info.ID); err != nil {
		t.Fatal(err)
	}
	waitProcess(t, p, info.ID)
	finish, err := boundary.Begin(context.Background(), "joined")
	if err != nil {
		t.Fatal(err)
	}
	info, err = p.Start(context.Background(), "read input")
	if err != nil {
		t.Fatal(err)
	}
	if err = finish(context.Background()); err == nil {
		t.Fatal("after capture accepted live writer")
	}
	pairs := boundary.Pairs()
	if len(pairs) != 1 || pairs[0].After != "" || pairs[0].Error == "" {
		t.Fatal(pairs)
	}
	if err = p.Cancel(info.ID); err != nil {
		t.Fatal(err)
	}
	waitProcess(t, p, info.ID)
}
