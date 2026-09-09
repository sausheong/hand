package tui

import (
	"context"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	"testing"
)

func TestStreamEvents_ForwardsEventsThenRunEnded(t *testing.T) {
	backend := applicationBackend{run: func(context.Context, string) (<-chan app.BackendEvent, error) {
		events := make(chan app.BackendEvent, 2)
		events <- app.BackendEvent{Text: "hi"}
		events <- app.BackendEvent{Kind: "usage", Done: true, Details: app.Details{UsageKnown: true, InputTokens: 1}}
		close(events)
		return events, nil
	}}
	stream, err := app.New(backend, app.Options{SessionID: "session", MaxIterations: 1}).Start(context.Background(), "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Cancel()
	var sequence uint64
	text, usage, terminal := 0, 0, 0
	for {
		msg := nextApplicationEvent(stream)().(applicationMsg)
		if msg.outcome != nil {
			terminal++
			if msg.event.Kind != "terminal" || msg.event.Sequence <= sequence || msg.outcome.Status != agentio.Completed {
				t.Fatal("invalid final event", msg)
			}
			break
		}
		for _, event := range append([]app.Event{msg.event}, msg.following...) {
			if event.Sequence <= sequence || event.SessionID != "session" || event.RunID != 1 {
				t.Fatal("event identity/order lost", event)
			}
			sequence = event.Sequence
			if event.Kind == "text" {
				if event.Text != "hi" {
					t.Fatal(event)
				}
				text++
			}
			if event.Kind == "usage" {
				if !event.Details.UsageKnown || event.Details.InputTokens != 1 {
					t.Fatal(event)
				}
				usage++
			}
		}
	}
	if text != 1 || usage != 1 || terminal != 1 {
		t.Fatal(text, usage, terminal)
	}
	select {
	case <-stream.Done:
	default:
		t.Fatal("terminal delivered before worker joined")
	}
}
