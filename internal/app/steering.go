package app

import (
	"context"
	"errors"
	"github.com/sausheong/hand/internal/agentio"
	"slices"

	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

// steeringSource is called by the owned Harness run at a joined tool boundary.
// Claimed text remains immutable until Harness confirms durable delivery. A
// failed acknowledgement can retry the same identity without changing its text.

func (s *Service) steeringSource(ctx context.Context) (*runtime.SteeringMessage, error) {
	s.mu.Lock()
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	if !s.active || s.state == Idle || s.state == Cancelling {
		s.mu.Unlock()
		return nil, nil
	}
	var selected QueuedInput
	for _, input := range s.inputs {
		if input.Queue == SteeringQueue && input.SessionID == s.options.SessionID {
			selected = input
			break
		}
	}
	if selected.ID == "" {
		s.mu.Unlock()
		return nil, nil
	}
	parsed, cached := s.steeringSnapshots[selected.ID]
	resolve := s.options.ResolveInput
	s.mu.Unlock()
	if !cached {
		parsed = agentio.PromptInput{Display: selected.Text, Prompt: selected.Text}
		if resolve != nil {
			var err error
			parsed, err = resolve(ctx, selected.Text)
			if err != nil {
				return nil, err
			}
		}
		parsed.Images = cloneSteeringImages(parsed.Images)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !s.active || s.state == Idle || s.state == Cancelling {
		return nil, nil
	}
	for i, current := range s.inputs {
		if current.Queue != SteeringQueue || current.SessionID != s.options.SessionID {
			continue
		}
		if current != selected {
			return nil, errors.New("steering input changed during attachment resolution")
		}
		if len(parsed.Images) > 0 && len(s.options.InputTypes) > 0 && !slices.Contains(s.options.InputTypes, "image") {
			return nil, errors.New("selected profile does not support steering images")
		}
		if s.steeringSnapshots == nil {
			s.steeringSnapshots = make(map[string]agentio.PromptInput)
		}
		s.steeringSnapshots[selected.ID] = parsed
		s.inputs[i].Claimed = true
		return &runtime.SteeringMessage{ID: selected.ID, Text: parsed.Prompt, Images: cloneSteeringImages(parsed.Images), Acknowledge: func() error {
			s.mu.Lock()
			defer s.mu.Unlock()
			for j, current := range s.inputs {
				if current.ID != selected.ID {
					continue
				}
				if !current.Claimed || current.Text != selected.Text || current.SessionID != selected.SessionID {
					return errors.New("steering claim changed before acknowledgement")
				}
				s.inputs = append(s.inputs[:j], s.inputs[j+1:]...)
				delete(s.steeringSnapshots, selected.ID)
				return nil
			}
			return nil
		}}, nil
	}
	return nil, errors.New("steering input removed during attachment resolution")
}

// reconcileSteering runs only after the Harness stream has closed and all its
// producers have joined. Flush must succeed before absence/presence is treated
// as authoritative. Validation precedes mutation so a conflict cannot partially
// change the queue. A failed flush leaves claims visible and immutable.
func (s *Service) reconcileSteering(sess *session.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	claimed := false
	for _, input := range s.inputs {
		if input.SessionID == s.options.SessionID && input.Claimed {
			claimed = true
			break
		}
	}
	if !claimed {
		return nil
	}
	if sess == nil || sess.ID != s.options.SessionID {
		return errors.New("steering reconciliation session identity mismatch")
	}
	if err := sess.Flush(); err != nil {
		return err
	}
	entries := sess.Entries()
	delivered := make(map[string]bool)
	for _, input := range s.inputs {
		if input.SessionID != sess.ID || !input.Claimed {
			continue
		}
		snapshot, ok := s.steeringSnapshots[input.ID]
		if !ok {
			snapshot = agentio.PromptInput{Prompt: input.Text}
		}
		expected := runtime.SteeringMessage{ID: input.ID, Text: snapshot.Prompt, Images: snapshot.Images}
		for _, entry := range entries {
			if entry.ID != "steering_"+input.ID {
				continue
			}
			matches, err := runtime.MatchesSteering(context.Background(), sess, expected, entry)
			if err != nil {
				return err
			}
			if !matches {
				return errors.New("steering reconciliation found conflicting delivered identity")
			}
			delivered[input.ID] = true
		}
	}
	retained := s.inputs[:0]
	for _, input := range s.inputs {
		if input.SessionID == sess.ID && input.Claimed {
			delete(s.steeringSnapshots, input.ID)
			if delivered[input.ID] {
				continue
			}
			input.Claimed = false
		}
		retained = append(retained, input)
	}
	s.inputs = retained
	return nil
}

func cloneSteeringImages(images []llm.ImageContent) []llm.ImageContent {
	if images == nil {
		return nil
	}
	out := make([]llm.ImageContent, len(images))
	for i, img := range images {
		out[i] = llm.ImageContent{MimeType: img.MimeType, Data: append([]byte(nil), img.Data...)}
	}
	return out
}
