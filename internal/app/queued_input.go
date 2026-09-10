package app

import (
	"context"
	"crypto/rand"
	"errors"
	"github.com/sausheong/hand/internal/agentio"
	"strings"
	"unicode/utf8"
)

type InputQueue string

const (
	SteeringQueue      InputQueue = "steering"
	FollowupQueue      InputQueue = "followup"
	MaxQueuedInputs               = 64
	MaxQueuedTextBytes            = 64 << 10
)

// QueuedInput is an immutable snapshot. IDs survive edits and failed admission.
// Entries belong to the selected session and are retained until delivery or removal.
type QueuedInput struct {
	ID        string
	SessionID string
	Queue     InputQueue
	Text      string
	Claimed   bool
}

func validQueuedText(text string) bool {
	return utf8.ValidString(text) && len(text) <= MaxQueuedTextBytes && strings.TrimSpace(text) != ""
}

func (s *Service) EnqueueInput(queue InputQueue, text string) (QueuedInput, error) {
	if queue != SteeringQueue && queue != FollowupQueue {
		return QueuedInput{}, errors.New("invalid input queue")
	}
	if !validQueuedText(text) {
		return QueuedInput{}, errors.New("queued input must be nonempty UTF-8 within the text limit")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.inputs) >= MaxQueuedInputs {
		return QueuedInput{}, errors.New("input queue is full")
	}
	input := QueuedInput{ID: rand.Text(), SessionID: s.options.SessionID, Queue: queue, Text: text}
	s.inputs = append(s.inputs, input)
	return input, nil
}

func (s *Service) QueuedInputs() []QueuedInput {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]QueuedInput, 0, len(s.inputs))
	for _, input := range s.inputs {
		if input.SessionID == s.options.SessionID {
			out = append(out, input)
		}
	}
	return out
}

func (s *Service) EditQueuedInput(id, text string) error {
	if !validQueuedText(text) {
		return errors.New("invalid queued input text")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.inputs {
		if s.inputs[i].ID == id && s.inputs[i].SessionID == s.options.SessionID {
			if s.inputs[i].Claimed {
				return errors.New("steering delivery is unresolved; input cannot be edited")
			}
			s.inputs[i].Text = text
			return nil
		}
	}
	return errors.New("queued input not found")
}

func (s *Service) RemoveQueuedInput(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, input := range s.inputs {
		if input.ID == id && input.SessionID == s.options.SessionID {
			if input.Claimed {
				return errors.New("steering delivery is unresolved; input cannot be removed")
			}
			s.inputs = append(s.inputs[:i], s.inputs[i+1:]...)
			return nil
		}
	}
	return errors.New("queued input not found")
}

// StartFollowup atomically admits the oldest follow-up after all active work,
// completion checks and terminal persistence have joined. Failed admission leaves
// the queue unchanged. Accepted input has become a run and is never auto-requeued:
// cancellation cannot silently duplicate a prompt already sent to the backend.

func (s *Service) StartFollowup(parent context.Context) (*Stream, QueuedInput, error) {
	s.mu.Lock()
	if err := parent.Err(); err != nil {
		s.mu.Unlock()
		return nil, QueuedInput{}, err
	}
	if s.active {
		s.mu.Unlock()
		return nil, QueuedInput{}, ErrBusy
	}
	var selected QueuedInput
	for _, input := range s.inputs {
		if input.SessionID == s.options.SessionID && input.Queue == FollowupQueue {
			selected = input
			break
		}
	}
	resolve := s.options.ResolveInput
	s.mu.Unlock()
	if selected.ID == "" {
		return nil, QueuedInput{}, errors.New("no queued follow-up")
	}
	// File I/O must not hold the owner lock. Recheck the selected identity/text
	// and queue order before admission so edits/removal cannot send stale input.
	parsed := agentio.PromptInput{Display: selected.Text, Prompt: selected.Text}
	if resolve != nil {
		var err error
		parsed, err = resolve(parent, selected.Text)
		if err != nil {
			return nil, QueuedInput{}, err
		}
	}
	s.mu.Lock()
	if err := parent.Err(); err != nil {
		s.mu.Unlock()
		return nil, QueuedInput{}, err
	}
	if s.active {
		s.mu.Unlock()
		return nil, QueuedInput{}, ErrBusy
	}
	for i, input := range s.inputs {
		if input.SessionID != s.options.SessionID || input.Queue != FollowupQueue {
			continue
		}
		if input != selected {
			s.mu.Unlock()
			return nil, QueuedInput{}, errors.New("queued input changed during attachment resolution; retry with the current entry")
		}
		ctx, cancel, runID, operationID, err := s.beginGoalLocked(parent, parsed.Images)
		if err != nil {
			s.mu.Unlock()
			return nil, QueuedInput{}, err
		}
		s.inputs = append(s.inputs[:i], s.inputs[i+1:]...)
		s.mu.Unlock()
		return s.startOwnedStream(ctx, cancel, runID, operationID, parsed.Prompt, parsed.Images), input, nil
	}
	s.mu.Unlock()
	return nil, QueuedInput{}, errors.New("queued input was removed during attachment resolution")
}
