package tui

import (
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/harness/runtime"
)

type fakeProgramSender struct {
	mu   sync.Mutex
	sent []tea.Msg
}

func (f *fakeProgramSender) Send(msg tea.Msg) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, msg)
}

func (f *fakeProgramSender) snapshot() []tea.Msg {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]tea.Msg, len(f.sent))
	copy(out, f.sent)
	return out
}

func TestStreamEvents_ForwardsEventsThenRunEnded(t *testing.T) {
	events := make(chan runtime.AgentEvent, 2)
	events <- runtime.AgentEvent{Type: runtime.EventTextDelta, Text: "hi"}
	events <- runtime.AgentEvent{Type: runtime.EventDone}
	close(events)

	sender := &fakeProgramSender{}
	done := make(chan struct{})
	go func() {
		StreamEvents(sender, events)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StreamEvents did not return after the channel closed")
	}

	got := sender.snapshot()
	if len(got) != 3 {
		t.Fatalf("sent %d messages, want 3 (2 events + runEndedMsg): %+v", len(got), got)
	}
	if ev, ok := got[0].(runtime.AgentEvent); !ok || ev.Text != "hi" {
		t.Errorf("first message = %+v, want AgentEvent{Text: \"hi\"}", got[0])
	}
	if ev, ok := got[1].(runtime.AgentEvent); !ok || ev.Type != runtime.EventDone {
		t.Errorf("second message = %+v, want AgentEvent{Type: EventDone}", got[1])
	}
	if _, ok := got[2].(runEndedMsg); !ok {
		t.Errorf("third message = %+v (%T), want runEndedMsg", got[2], got[2])
	}
}
