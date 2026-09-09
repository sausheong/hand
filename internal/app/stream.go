package app

import (
	"context"
	"github.com/sausheong/hand/internal/agentio"

	"github.com/sausheong/harness/llm"
)

const EventQueueCapacity = 64
const MaxEventTextBytes = 16 << 10

// Stream separates bounded progress delivery from the single terminal result.
// Read Events through closure before displaying Terminal to preserve ordering.
// Terminal and Done do not require the progress consumer to keep draining after
// cancellation. Close/Cancel must be called when a client disconnects.
type Stream struct {
	identity agentio.RunIdentity
	Events   <-chan Event
	Terminal <-chan Event
	Done     <-chan struct{}
	cancel   func() bool
	outcome  agentio.RunOutcome
	err      error
	final    Event
}

func (r *Stream) Identity() agentio.RunIdentity { return r.identity }

func (r *Stream) Cancel() bool                      { return r.cancel() }
func (r *Stream) Close()                            { r.cancel() }
func (r *Stream) Wait() (agentio.RunOutcome, error) { <-r.Done; return r.outcome, r.err }

// FinalEvent returns the immutable terminal snapshot after all run work joins.
func (r *Stream) FinalEvent() Event { <-r.Done; return r.final }

// Start claims ownership synchronously, so an accepted run cannot race another
// Start or a configuration operation while waiting for its goroutine to start.
// Backpressure bounds the progress queue; cancellation unblocks the producer,
// which still joins its backend/checker before publishing the terminal result.
func (s *Service) Start(parent context.Context, prompt string, images []llm.ImageContent) (*Stream, error) {
	ctx, cancel, runID, operationID, err := s.beginGoal(parent, images)
	if err != nil {
		return nil, err
	}
	return s.startOwnedStream(ctx, cancel, runID, operationID, prompt, images), nil
}

func (s *Service) startOwnedStream(ctx context.Context, cancel context.CancelFunc, runID, operationID uint64, prompt string, images []llm.ImageContent) *Stream {
	ownedImages := make([]llm.ImageContent, len(images))
	for i, image := range images {
		ownedImages[i] = llm.ImageContent{MimeType: image.MimeType, Data: append([]byte(nil), image.Data...)}
	}
	events := make(chan Event, EventQueueCapacity)
	terminal := make(chan Event, 1)
	done := make(chan struct{})
	stream := &Stream{identity: agentio.IdentityFromContext(ctx), Events: events, Terminal: terminal, Done: done, cancel: func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.active || s.operationID != operationID || s.state == Idle {
			return false
		}
		s.state = Cancelling
		cancel()
		return true
	}}
	go func() {
		var final Event
		var accounting Details
		hasAccounting := false
		stream.outcome, stream.err = s.executeOwned(ctx, cancel, runID, prompt, ownedImages, func(event Event) {
			if event.Kind == "session_usage" {
				accounting = event.Details
				hasAccounting = true
			}
			if event.Kind == "terminal" {
				final = event
				return
			}
			if ctx.Err() != nil {
				return
			}
			select {
			case events <- event:
			case <-ctx.Done():
			}
		})
		// executeOwned has released ownership and joined all run workers here.
		if hasAccounting {
			final.Details = accounting
		}
		stream.final = final
		close(events)
		terminal <- final
		close(terminal)
		close(done)
	}()
	return stream
}
