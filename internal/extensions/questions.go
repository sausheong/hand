package extensions

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"sort"
	"sync"

	"github.com/sausheong/hand/extension/protocol"
)

const MaxPendingQuestions = 8

var ErrQuestionClosed = errors.New("extension question is no longer pending")

type PendingQuestion struct {
	Token     string            `json:"token"`
	Extension string            `json:"extension"`
	Question  protocol.Question `json:"question"`
	Sequence  uint64            `json:"sequence"`
}
type questionResult struct {
	answer protocol.Answer
	err    error
}
type waitingQuestion struct {
	view   PendingQuestion
	ctx    context.Context
	result chan questionResult
}

// Questions is host-owned session UI state. A token identifies one live question,
// independently of a peer's reusable question ID. No permission grants live here.
type Questions struct {
	mu      sync.Mutex
	pending map[string]*waitingQuestion
	next    uint64
	closed  bool
}

func NewQuestions() *Questions { return &Questions{pending: make(map[string]*waitingQuestion)} }
func cloneQuestion(q protocol.Question) protocol.Question {
	q.Options = append([]protocol.Option(nil), q.Options...)
	return q
}
func (q *Questions) Ask(ctx context.Context, extension string, question protocol.Question) (protocol.Answer, error) {
	if err := (protocol.Hello{Version: 1, Name: extension}).Validate(nil); err != nil {
		return protocol.Answer{}, err
	}
	if err := question.Validate(); err != nil {
		return protocol.Answer{}, err
	}
	if err := ctx.Err(); err != nil {
		return protocol.Answer{}, err
	}
	question = cloneQuestion(question)
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return protocol.Answer{}, ErrQuestionClosed
	}
	if len(q.pending) >= MaxPendingQuestions {
		q.mu.Unlock()
		return protocol.Answer{}, errors.New("too many pending extension questions")
	}
	q.next++
	token := rand.Text()
	for q.pending[token] != nil {
		token = rand.Text()
	}
	waiting := &waitingQuestion{view: PendingQuestion{Token: token, Extension: extension, Question: question, Sequence: q.next}, ctx: ctx, result: make(chan questionResult, 1)}
	q.pending[token] = waiting
	q.mu.Unlock()
	defer func() { q.mu.Lock(); delete(q.pending, token); q.mu.Unlock() }()
	select {
	case result := <-waiting.result:
		if ctx.Err() != nil {
			return protocol.Answer{}, context.Cause(ctx)
		}
		return result.answer, result.err
	case <-ctx.Done():
		return protocol.Answer{}, context.Cause(ctx)
	}
}
func (q *Questions) Pending() []PendingQuestion {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := []PendingQuestion{}
	for _, waiting := range q.pending {
		if waiting.ctx.Err() != nil {
			continue
		}
		view := waiting.view
		view.Question = cloneQuestion(view.Question)
		out = append(out, view)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	return out
}

// Respond accepts only an answer to the exact live host token. Replays and
// answers after cancellation cannot affect a later question with the same ID.
func (q *Questions) Respond(token string, answer protocol.Answer) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	waiting := q.pending[token]
	if waiting == nil || waiting.ctx.Err() != nil || q.closed {
		return ErrQuestionClosed
	}
	if err := waiting.view.Question.ValidateAnswer(answer); err != nil {
		return err
	}
	delete(q.pending, token)
	waiting.result <- questionResult{answer: answer}
	return nil
}
func (q *Questions) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	for token, waiting := range q.pending {
		delete(q.pending, token)
		waiting.result <- questionResult{err: ErrQuestionClosed}
	}
}

// Handler captures the host-admitted extension identity, never an identity
// supplied in callback params. Question answers must not be used as tool grants.
func (q *Questions) Handler(extension string) CallbackHandler {
	return func(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, *protocol.Error) {
		if method != "user.question" {
			return nil, &protocol.Error{Code: "capability_denied", Message: "question handler cannot access host resources"}
		}
		var question protocol.Question
		if err := protocol.DecodePayload(params, &question); err != nil {
			return nil, &protocol.Error{Code: "invalid_params", Message: err.Error()}
		}
		answer, err := q.Ask(ctx, extension, question)
		if err != nil {
			return nil, &protocol.Error{Code: "question_unavailable", Message: err.Error()}
		}
		raw, err := json.Marshal(answer)
		if err != nil {
			return nil, &protocol.Error{Code: "question_unavailable", Message: err.Error()}
		}
		return raw, nil
	}
}
