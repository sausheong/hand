package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/checkpoints"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func TestCheckpointJournalPersistsRunPair(t *testing.T) {
	sessions := session.NewStore(t.TempDir())
	if err := sessions.Create("hand", "key"); err != nil {
		t.Fatal(err)
	}
	sess, err := sessions.LoadExclusive("hand", "key")
	if err != nil {
		t.Fatal(err)
	}
	backend := &HarnessBackend{Runtime: &runtime.Runtime{Session: sess}}
	work := t.TempDir()
	store, err := checkpoints.OpenStore(filepath.Join(t.TempDir(), "snapshots"), work, checkpoints.DefaultStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	boundary := &WorkspaceCheckpoints{Workspace: work, Store: store, Limits: checkpoints.DefaultLimits()}
	service := New(backend, Options{MaxIterations: 1})
	if err = service.ConfigureCheckpoints(boundary); err != nil {
		t.Fatal(err)
	}
	finish, err := boundary.Begin(context.Background(), "durable-run")
	if err != nil {
		t.Fatal(err)
	}
	records, err := ReadSessionCheckpoints(sess)
	if err != nil || len(records) != 1 || records[0].Finish != nil {
		t.Fatal(records, err)
	}
	if err = finish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = sess.Close(); err != nil {
		t.Fatal(err)
	}
	sess, err = sessions.LoadExclusive("hand", "key")
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	records, err = ReadSessionCheckpoints(sess)
	if err != nil || len(records) != 1 || records[0].Finish == nil || records[0].Start.Before != records[0].Finish.Before || records[0].Finish.After == "" {
		t.Fatal(records, err)
	}
	if _, err = store.Load(context.Background(), records[0].Finish.After); err != nil {
		t.Fatal(err)
	}
}
func TestCheckpointJournalFailurePreventsRunAdmission(t *testing.T) {
	work := t.TempDir()
	store, err := checkpoints.OpenStore(filepath.Join(t.TempDir(), "snapshots"), work, checkpoints.DefaultStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	injected := errors.New("journal unavailable")
	boundary := &WorkspaceCheckpoints{Workspace: work, Store: store, Limits: checkpoints.DefaultLimits(), Record: func(string, CheckpointPair) error { return injected }}
	if finish, err := boundary.Begin(context.Background(), "run"); finish != nil || !errors.Is(err, injected) {
		t.Fatal("journal failure admitted execution", err)
	}
}
func TestCheckpointJournalRejectsUnmatchedAndChangedFinish(t *testing.T) {
	sessions := session.NewStore(t.TempDir())
	if err := sessions.Create("hand", "key"); err != nil {
		t.Fatal(err)
	}
	sess, err := sessions.LoadExclusive("hand", "key")
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	backend := &HarnessBackend{Runtime: &runtime.Runtime{Session: sess}}
	pair := CheckpointPair{RunID: "run", Before: strings.Repeat("a", 64), After: strings.Repeat("b", 64)}
	if err = backend.RecordCheckpoint("finish", pair); err == nil {
		t.Fatal("unmatched finish accepted")
	}
	pair.After = ""
	if err = backend.RecordCheckpoint("start", pair); err != nil {
		t.Fatal(err)
	}
	pair.Before = strings.Repeat("c", 64)
	pair.After = strings.Repeat("b", 64)
	if err = backend.RecordCheckpoint("finish", pair); err == nil {
		t.Fatal("changed before-image accepted")
	}
	records, err := ReadSessionCheckpoints(sess)
	if err != nil || len(records) != 1 || records[0].Finish != nil {
		t.Fatal(records, err)
	}
}
