package extensions

import (
	"context"
	"errors"
	"github.com/sausheong/hand/extension/protocol"
	"testing"
	"time"
)

func waitQuestions(t *testing.T, q *Questions, count int) []PendingQuestion {
	t.Helper()
	until := time.Now().Add(2 * time.Second)
	for {
		pending := q.Pending()
		if len(pending) == count {
			return pending
		}
		if time.Now().After(until) {
			t.Fatalf("pending=%d want=%d", len(pending), count)
		}
		time.Sleep(time.Millisecond)
	}
}
func TestQuestionsTokensCancellationAndSnapshotIsolation(t *testing.T) {
	q := NewQuestions()
	defer q.Close()
	question := protocol.Question{ID: "repeat", Title: "Choose mode", Options: []protocol.Option{{ID: "one", Label: "First"}}}
	ask := func(ctx context.Context) chan error {
		done := make(chan error, 1)
		go func() { _, err := q.Ask(ctx, "fixture", question); done <- err }()
		return done
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := ask(ctx)
	first := waitQuestions(t, q, 1)[0]
	first.Question.Options[0].ID = "mutated"
	if err := q.Respond(first.Token, protocol.Answer{ID: question.ID, Choice: "mutated"}); err == nil {
		t.Fatal("snapshot mutation changed valid options")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	next := ask(context.Background())
	second := waitQuestions(t, q, 1)[0]
	if first.Token == second.Token {
		t.Fatal("question token reused")
	}
	if err := q.Respond(first.Token, protocol.Answer{ID: question.ID, Choice: "one"}); !errors.Is(err, ErrQuestionClosed) {
		t.Fatal("stale response accepted", err)
	}
	if err := q.Respond(second.Token, protocol.Answer{ID: question.ID, Choice: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := <-next; err != nil {
		t.Fatal(err)
	}
	if err := q.Respond(second.Token, protocol.Answer{ID: question.ID, Choice: "one"}); !errors.Is(err, ErrQuestionClosed) {
		t.Fatal("answer replay accepted", err)
	}
}
func TestQuestionsCapacityAndCloseJoinWaiters(t *testing.T) {
	q := NewQuestions()
	question := protocol.Question{ID: "q", Title: "Free text", AllowFreeText: true}
	done := make(chan error, MaxPendingQuestions)
	for i := 0; i < MaxPendingQuestions; i++ {
		go func() { _, err := q.Ask(context.Background(), "fixture", question); done <- err }()
	}
	waitQuestions(t, q, MaxPendingQuestions)
	if _, err := q.Ask(context.Background(), "fixture", question); err == nil {
		t.Fatal("question limit ignored")
	}
	q.Close()
	for i := 0; i < MaxPendingQuestions; i++ {
		select {
		case err := <-done:
			if !errors.Is(err, ErrQuestionClosed) {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("waiter leaked")
		}
	}
	if len(q.Pending()) != 0 {
		t.Fatal("closed questions remain")
	}
}
