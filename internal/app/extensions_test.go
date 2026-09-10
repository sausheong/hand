package app

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func TestApplicationExtensionOwnershipQuestionsAndSessionBinding(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "note")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./examples/extensions/go-task-note")
	build.Dir = "../.."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, output)
	}
	disk := session.NewStore(t.TempDir())
	if err := disk.Create("hand", "one"); err != nil {
		t.Fatal(err)
	}
	if err := disk.Create("hand", "two"); err != nil {
		t.Fatal(err)
	}
	one, err := disk.LoadExclusive("hand", "one")
	if err != nil {
		t.Fatal(err)
	}
	defer one.Close()
	two, err := disk.LoadExclusive("hand", "two")
	if err != nil {
		t.Fatal(err)
	}
	defer two.Close()
	controller := &Controller{Rt: &runtime.Runtime{Session: one}}
	review, err := extensions.ReviewHostLaunch(ctx, extensions.LaunchConfig{Name: "task-note", Executable: binary, Workspace: t.TempDir(), Capabilities: []string{"commands", "questions", "state", "context.transform"}})
	if err != nil {
		t.Fatal(err)
	}
	factory, err := extensions.NewHostFactory(filepath.Join(t.TempDir(), "private"), []extensions.LaunchReview{review}, func(context.Context, extensions.LaunchReview) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	identities := map[string]string{"task-note": "installed/task-note"}
	host, err := NewExtensionHost(ctx, controller, factory, identities)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	identities["task-note"] = "mutated"
	if _, err = host.Reload(ctx, []extensions.Specification{review.Specification}); err != nil {
		t.Fatal(err)
	}
	start := func() chan error {
		done := make(chan error, 1)
		go func() { _, err := host.Execute(ctx, "task-note", "note", ""); done <- err }()
		return done
	}
	pending := func() extensions.PendingQuestion {
		t.Helper()
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			if q := host.Pending(); len(q) > 0 {
				return q[0]
			}
			select {
			case <-ctx.Done():
				t.Fatal("question did not arrive")
			case <-ticker.C:
			}
		}
	}
	done := start()
	question := pending()
	if _, err = host.Reload(ctx, nil); !errors.Is(err, ErrBusy) {
		t.Fatalf("reload during question: %v", err)
	}
	if _, _, err = controller.owner().reserve(ctx, Idle); !errors.Is(err, ErrBusy) {
		t.Fatalf("session ownership available during question: %v", err)
	}
	if err = host.Answer(question.Token, protocol.Answer{ID: question.Question.ID, Text: "first session"}); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if err = host.Answer(question.Token, protocol.Answer{ID: question.Question.ID, Text: "replay"}); err == nil {
		t.Fatal("answer replay accepted")
	}
	state, _ := extensions.NewStateStore(one, "installed/task-note")
	saved, err := state.Get(ctx)
	if err != nil || string(saved.Data) != `{"note":"first session"}` {
		t.Fatalf("state: %+v %v", saved, err)
	}
	_, release, err := controller.owner().reserve(ctx, Idle)
	if err != nil {
		t.Fatal(err)
	}
	controller.mu.Lock()
	controller.Rt.Session = two
	controller.mu.Unlock()
	release()
	done = start()
	question = pending()
	if err = host.Answer(question.Token, protocol.Answer{ID: question.Question.ID, Text: "second session"}); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	state, _ = extensions.NewStateStore(two, "installed/task-note")
	saved, err = state.Get(ctx)
	if err != nil || saved.Revision != 1 || string(saved.Data) != `{"note":"second session"}` {
		t.Fatalf("selected session state: %+v %v", saved, err)
	}
	done = start()
	question = pending()
	if !controller.owner().Cancel() {
		t.Fatal("owner did not cancel")
	}
	if err = <-done; err == nil {
		t.Fatal("cancelled command succeeded")
	}
	if len(host.Pending()) != 0 {
		t.Fatal("cancelled question remains")
	}
	if err = host.Answer(question.Token, protocol.Answer{ID: question.Question.ID, Text: "late"}); err == nil {
		t.Fatal("cancelled answer accepted")
	}
	_, release, err = controller.owner().reserve(ctx, Idle)
	if err != nil {
		t.Fatalf("owner not released after cancellation: %v", err)
	}
	release()
}
