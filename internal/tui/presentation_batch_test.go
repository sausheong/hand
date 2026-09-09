package tui

import (
	"github.com/sausheong/hand/internal/app"
	"strings"
	"testing"
)

func TestPresentationBatchStopsAtControlBoundary(t *testing.T) {
	events := make(chan app.Event, 8)
	for _, event := range []app.Event{{Kind: "text", Text: "one"}, {Kind: "text", Text: "two"}, {Kind: "approval_required", ApprovalID: "ask", Text: "preview", Details: app.Details{ToolName: "bash"}}, {Kind: "text", Text: "later"}} {
		events <- event
	}
	stream := &app.Stream{Events: events}
	msg := nextApplicationEvent(stream)().(applicationMsg)
	if msg.event.Text != "one" || len(msg.following) != 2 || msg.following[1].Kind != "approval_required" || len(events) != 1 {
		t.Fatal("control boundary crossed", msg, len(events))
	}
	m := NewModel(nil, t.TempDir())
	m.running = true
	m.activeStream = stream
	m.handleApplicationMessage(msg)
	if m.streamBuf.String() != "onetwo" || m.pending == nil || m.pendingApprovalID != "ask" {
		t.Fatal("batch lost text or delayed approval")
	}
}
func TestPresentationBatchBoundsAndExactText(t *testing.T) {
	events := make(chan app.Event, 100)
	for i := 0; i < 100; i++ {
		events <- app.Event{Kind: "text", Text: "x"}
	}
	stream := &app.Stream{Events: events}
	m := NewModel(nil, t.TempDir())
	m.running = true
	m.activeStream = stream
	batches := 0
	for len(events) > 0 {
		msg := nextApplicationEvent(stream)().(applicationMsg)
		if len(msg.following)+1 > maxPresentationEvents {
			t.Fatal("unbounded batch")
		}
		m.handleApplicationMessage(msg)
		batches++
	}
	if batches != 4 || m.streamBuf.String() != strings.Repeat("x", 100) {
		t.Fatal("batch dropped or duplicated text", batches)
	}
	large := make(chan app.Event, 3)
	for i := 0; i < 3; i++ {
		large <- app.Event{Kind: "text", Text: strings.Repeat("z", app.MaxEventTextBytes)}
	}
	msg := nextApplicationEvent(&app.Stream{Events: large})().(applicationMsg)
	if len(msg.following) != 0 || len(large) != 2 {
		t.Fatal("byte bound ignored")
	}
}
func TestPresentationBatchNeverWaitsForAnotherDelta(t *testing.T) {
	events := make(chan app.Event, 1)
	events <- app.Event{Kind: "text", Text: "immediate"}
	msg := nextApplicationEvent(&app.Stream{Events: events})().(applicationMsg)
	if msg.event.Text != "immediate" || len(msg.following) != 0 {
		t.Fatal("unexpected batch")
	}
}
