package app

import (
	"context"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/agentio"
)

func TestCancelledFullQueueRetainsTerminalAccounting(t *testing.T) {
	backend := &backendFixture{run: func(ctx context.Context, _ string) (<-chan BackendEvent, error) {
		events := make(chan BackendEvent, EventQueueCapacity+2)
		for i := 0; i < EventQueueCapacity+1; i++ {
			events <- BackendEvent{Text: "queued before cancellation"}
		}
		go func() {
			<-ctx.Done()
			events <- BackendEvent{Kind: "session_usage", Details: Details{UsageKnown: true, InputTokens: 123, UsageRequests: 2, UsageUnknown: 1}}
			close(events)
		}()
		return events, nil
	}}
	service := New(backend, Options{SessionID: "session", MaxIterations: 1})
	stream, err := service.Start(context.Background(), "go", nil)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for len(stream.Events) < EventQueueCapacity {
		select {
		case <-ticker.C:
		case <-deadline:
			stream.Cancel()
			t.Fatal("queue never filled")
		}
	}
	stream.Cancel()
	select {
	case <-stream.Done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancel blocked on full progress queue")
	}
	outcome, err := stream.Wait()
	if err != nil || outcome.Status != agentio.Cancelled {
		t.Fatal(outcome, err)
	}
	final := stream.FinalEvent()
	if !final.Details.UsageKnown || final.Details.InputTokens != 123 || final.Details.UsageRequests != 2 || final.Details.UsageUnknown != 1 {
		t.Fatal("terminal accounting lost", final)
	}
	wire := <-stream.Terminal
	if wire.Details != final.Details {
		t.Fatal("terminal API snapshots differ")
	}
}
