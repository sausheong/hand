package app

import (
	"context"
	"errors"

	"github.com/sausheong/hand/internal/agentio"
)

// StartInput resolves a submitted prompt outside the owner lock, then admits
// the immutable result only if its selected session is unchanged. Capability
// validation occurs at admission against the currently selected model profile.
func (s *Service) StartInput(parent context.Context, text string) (*Stream, error) {
	if !validQueuedText(text) {
		return nil, errors.New("prompt must be nonempty UTF-8 within 64 KiB")
	}
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		return nil, ErrBusy
	}
	resolve, sessionID := s.options.ResolveInput, s.options.SessionID
	s.mu.Unlock()
	parsed := agentio.PromptInput{Display: text, Prompt: text}
	if resolve != nil {
		var err error
		parsed, err = resolve(parent, text)
		if err != nil {
			return nil, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := parent.Err(); err != nil {
		return nil, err
	}
	if s.options.SessionID != sessionID {
		return nil, errors.New("selected session changed during input resolution")
	}
	ctx, cancel, runID, operationID, err := s.beginGoalLocked(parent, parsed.Images)
	if err != nil {
		return nil, err
	}
	return s.startOwnedStream(ctx, cancel, runID, operationID, parsed.Prompt, parsed.Images), nil
}
