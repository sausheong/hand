package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/llm"
)

type backendFixture struct {
	run    func(context.Context, string) (<-chan BackendEvent, error)
	reason string
}

func (b *backendFixture) Run(ctx context.Context, prompt string, _ []llm.ImageContent) (<-chan BackendEvent, error) {
	return b.run(ctx, prompt)
}
func (b *backendFixture) StopReason() string { return b.reason }
func completedStream() <-chan BackendEvent {
	ch := make(chan BackendEvent, 2)
	ch <- BackendEvent{Text: "answer"}
	ch <- BackendEvent{Done: true}
	close(ch)
	return ch
}

func TestGoalEventOrderingAndFinalIterationVerification(t *testing.T) {
	var prompts []string
	backend := &backendFixture{run: func(_ context.Context, prompt string) (<-chan BackendEvent, error) {
		prompts = append(prompts, prompt)
		return completedStream(), nil
	}}
	checks := 0
	service := New(backend, Options{SessionID: "session", MaxIterations: 2, Check: func(_ context.Context, _ string, iteration int) agentio.GoalLoopOutcome {
		checks++
		if iteration == 1 {
			return agentio.GoalLoopOutcome{Continue: true, NextPrompt: "fix remaining issue"}
		}
		return agentio.GoalLoopOutcome{Verified: true}
	}})
	var events []Event
	outcome, err := service.Execute(context.Background(), "initial", nil, func(event Event) { events = append(events, event) })
	if err != nil || outcome.Status != agentio.Completed || !outcome.Verified || checks != 2 {
		t.Fatalf("%+v checks=%d err=%v", outcome, checks, err)
	}
	if len(prompts) != 2 || prompts[1] != "fix remaining issue" {
		t.Fatal(prompts)
	}
	terminals := 0
	for i, event := range events {
		if event.Sequence != uint64(i+1) || event.SessionID != "session" || event.RunID != 1 || event.Timestamp.IsZero() {
			t.Fatalf("invalid event %+v", event)
		}
		if i > 0 && event.Timestamp.Before(events[i-1].Timestamp) {
			t.Fatal("event timestamp regressed")
		}
		if event.Kind == "terminal" {
			terminals++
			if i != len(events)-1 || event.State != Idle || !event.Verified {
				t.Fatalf("early/unverified terminal %+v", event)
			}
		}
	}
	if terminals != 1 || service.Snapshot().State != Idle {
		t.Fatalf("terminals=%d snapshot=%+v", terminals, service.Snapshot())
	}
}

func TestCancellationRetainsOwnershipUntilBackendJoins(t *testing.T) {
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var joined atomic.Bool
	backend := &backendFixture{run: func(ctx context.Context, _ string) (<-chan BackendEvent, error) {
		events := make(chan BackendEvent)
		go func() {
			close(started)
			<-ctx.Done()
			<-release
			events <- BackendEvent{Text: "late text"}
			joined.Store(true)
			close(events)
		}()
		return events, nil
	}}
	service := New(backend, Options{SessionID: "session", MaxIterations: 1})
	var result agentio.RunOutcome
	var captured []Event
	go func() {
		defer close(finished)
		result, _ = service.Execute(context.Background(), "first", nil, func(event Event) {
			captured = append(captured, event)
			if event.Kind == "terminal" && !joined.Load() {
				t.Error("terminal preceded backend join")
			}
		})
	}()
	<-started
	if !service.Cancel() || service.Snapshot().State != Cancelling {
		t.Fatal("cancel did not retain active state")
	}
	_, err := service.Execute(context.Background(), "overlap", nil, func(Event) { t.Error("rejected run emitted event") })
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("overlapping operation accepted: %v", err)
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled operation did not join")
	}
	if result.Status != agentio.Cancelled || service.Cancel() {
		t.Fatalf("outcome %+v", result)
	}
	for _, event := range captured {
		if event.Kind == "text" {
			t.Fatal("late cancelled text published")
		}
	}
}

func TestCancellationJoinsCompletionChecker(t *testing.T) {
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	backend := &backendFixture{run: func(context.Context, string) (<-chan BackendEvent, error) { return completedStream(), nil }}
	service := New(backend, Options{SessionID: "session", MaxIterations: 1, Check: func(ctx context.Context, _ string, _ int) agentio.GoalLoopOutcome {
		close(started)
		<-ctx.Done()
		<-release
		return agentio.GoalLoopOutcome{Verified: true}
	}})
	var outcome agentio.RunOutcome
	go func() { defer close(finished); outcome, _ = service.Execute(context.Background(), "first", nil, nil) }()
	<-started
	if service.Snapshot().State != CheckingCompletion {
		t.Fatal(service.Snapshot())
	}
	service.Cancel()
	_, err := service.Execute(context.Background(), "overlap", nil, nil)
	if !errors.Is(err, ErrBusy) {
		t.Fatal("checker cancellation released ownership early")
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("checker did not join")
	}
	if outcome.Status != agentio.Cancelled || outcome.Verified {
		t.Fatalf("cancelled check reported success %+v", outcome)
	}
}

func TestMalformedBackendCannotComplete(t *testing.T) {
	for _, tc := range []struct {
		name   string
		run    func(context.Context, string) (<-chan BackendEvent, error)
		reason string
	}{
		{"nil", func(context.Context, string) (<-chan BackendEvent, error) { return nil, nil }, "missing_event_stream"},
		{"start error", func(context.Context, string) (<-chan BackendEvent, error) { return nil, errors.New("fixture") }, "run_start_failed"},
		{"missing done", func(context.Context, string) (<-chan BackendEvent, error) {
			ch := make(chan BackendEvent)
			close(ch)
			return ch, nil
		}, "missing_completion"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := New(&backendFixture{run: tc.run}, Options{SessionID: "session", MaxIterations: 1})
			count := 0
			outcome, err := service.Execute(context.Background(), "first", nil, func(e Event) {
				if e.Kind == "terminal" {
					count++
				}
			})
			if err != nil || outcome.Status != agentio.InfrastructureError || outcome.Reason != tc.reason || count != 1 {
				t.Fatalf("%+v %v terminals=%d", outcome, err, count)
			}
		})
	}
}

func TestCompletionCommitRejectsLateCancelAndAdvancesIdentity(t *testing.T) {
	backend := &backendFixture{run: func(context.Context, string) (<-chan BackendEvent, error) { return completedStream(), nil }}
	service := New(backend, Options{SessionID: "session", MaxIterations: 1})
	var lastSequence uint64
	for run := uint64(1); run <= 2; run++ {
		result, err := service.Execute(context.Background(), "prompt", nil, func(event Event) {
			if event.Sequence <= lastSequence || event.RunID != run {
				t.Fatalf("identity/order %+v", event)
			}
			lastSequence = event.Sequence
			if event.Kind == "terminal" && service.Cancel() {
				t.Error("cancellation accepted after completion committed")
			}
		})
		if err != nil || result.Status != agentio.Completed {
			t.Fatalf("%+v %v", result, err)
		}
	}
}
