package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
)

func TestFollowupWaitsForCompletionAndKeepsQueueIntent(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	var prompts []string
	backend := &backendFixture{run: func(_ context.Context, prompt string) (<-chan BackendEvent, error) {
		mu.Lock()
		prompts = append(prompts, prompt)
		mu.Unlock()
		return completedStream(), nil
	}}
	owner := New(backend, Options{SessionID: "s", MaxIterations: 1, Check: func(context.Context, string, int) agentio.GoalLoopOutcome {
		once.Do(func() { close(entered); <-release })
		return agentio.GoalLoopOutcome{}
	}})
	steering, _ := owner.EnqueueInput(SteeringQueue, "correct direction")
	first, _ := owner.EnqueueInput(FollowupQueue, "first")
	second, _ := owner.EnqueueInput(FollowupQueue, "second")
	stream, err := owner.Start(context.Background(), "initial", nil)
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	if _, _, err := owner.StartFollowup(context.Background()); !errors.Is(err, ErrBusy) {
		t.Fatalf("follow-up bypassed completion check: %v", err)
	}
	if err := owner.EditQueuedInput(first.ID, "edited first"); err != nil {
		t.Fatal(err)
	}
	if err := owner.RemoveQueuedInput(second.ID); err != nil {
		t.Fatal(err)
	}
	close(release)
	for range stream.Events {
	}
	stream.Wait()
	next, delivered, err := owner.StartFollowup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if delivered.ID != first.ID || delivered.Text != "edited first" {
		t.Fatal(delivered)
	}
	for range next.Events {
	}
	next.Wait()
	queue := owner.QueuedInputs()
	if len(queue) != 1 || queue[0].ID != steering.ID {
		t.Fatal(queue)
	}
	if _, _, err := owner.StartFollowup(context.Background()); err == nil {
		t.Fatal("delivered duplicate")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(prompts) != 2 || prompts[1] != "edited first" {
		t.Fatal(prompts)
	}
}

func TestQueuedAdmissionFailureAndSnapshotIsolation(t *testing.T) {
	owner := New(nil, Options{SessionID: "s"})
	input, err := owner.EnqueueInput(FollowupQueue, "keep me")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := owner.StartFollowup(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, _, err := owner.StartFollowup(context.Background()); err == nil {
		t.Fatal("missing backend accepted")
	}
	snapshot := owner.QueuedInputs()
	snapshot[0].Text = "mutated"
	if got := owner.QueuedInputs(); len(got) != 1 || got[0] != input {
		t.Fatal(got)
	}
	if err := owner.EditQueuedInput(input.ID, ""); err == nil {
		t.Fatal("empty edit accepted")
	}
	if owner.Snapshot().RunID != 0 {
		t.Fatal("failed admission allocated run")
	}
}

func TestInputQueuesBoundedAndSeparate(t *testing.T) {
	owner := New(nil, Options{})
	for _, text := range []string{" ", string([]byte{255}), strings.Repeat("x", MaxQueuedTextBytes+1)} {
		if _, err := owner.EnqueueInput(FollowupQueue, text); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	if _, err := owner.EnqueueInput("invalid", "text"); err == nil {
		t.Fatal("unknown queue accepted")
	}
	for range MaxQueuedInputs {
		if _, err := owner.EnqueueInput(SteeringQueue, "text"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := owner.EnqueueInput(FollowupQueue, "overflow"); err == nil {
		t.Fatal("unbounded queue")
	}
	input := owner.QueuedInputs()[0]
	if err := owner.RemoveQueuedInput(input.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.EnqueueInput(FollowupQueue, "replacement"); err != nil {
		t.Fatal(err)
	}
	if err := owner.RemoveQueuedInput(input.ID); err == nil {
		t.Fatal("removed twice")
	}
}
