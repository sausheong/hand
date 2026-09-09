//go:build darwin || linux

package rpc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/llm"
)

type automaticBackend struct {
	entered chan string
	release chan struct{}
	before  func(string)
}

func (*automaticBackend) StopReason() string { return "" }
func (b *automaticBackend) Run(ctx context.Context, text string, _ []llm.ImageContent) (<-chan app.BackendEvent, error) {
	if b.before != nil {
		b.before(text)
	}
	b.entered <- text
	out := make(chan app.BackendEvent, 1)
	go func() {
		defer close(out)
		if text == "parent" {
			select {
			case <-b.release:
			case <-ctx.Done():
				return
			}
		}
		out <- app.BackendEvent{Done: true}
	}()
	return out, nil
}
func waitRPCIdle(t *testing.T, d *Dispatcher) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		idle := d.active == nil
		d.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("dispatcher did not become idle")
}
func TestAutomaticFollowupsFIFOAndDurablePredecessor(t *testing.T) {
	l, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	b := &automaticBackend{entered: make(chan string, 4), release: make(chan struct{})}
	d := NewDispatcher(app.New(b, app.Options{SessionID: "session", MaxIterations: 1}), l)
	defer d.Close()
	call := func(id, method, params string) json.RawMessage {
		t.Helper()
		r := d.Dispatch(context.Background(), rpcRequest(id, method, params))
		if r.Error != nil {
			t.Fatal(r.Error)
		}
		return r.Result
	}
	call("h", "hello", `{}`)
	call("p1", "prompt", `{"text":"parent"}`)
	if got := <-b.entered; got != "parent" {
		t.Fatal(got)
	}
	var first, second struct {
		RequestID string `json:"request_id"`
	}
	json.Unmarshal(call("q1", "followup.enqueue", `{"text":"first","auto":true}`), &first)
	json.Unmarshal(call("q2", "followup.enqueue", `{"text":"second","auto":true}`), &second)
	violations := make(chan string, 2)
	b.before = func(text string) {
		id := "p1"
		if text == "second" {
			id = first.RequestID
		}
		r, e := l.Lookup(id)
		if e != nil || r.State != "completed" {
			violations <- text
		}
	}
	close(b.release)
	for _, want := range []string{"first", "second"} {
		select {
		case got := <-b.entered:
			if got != want {
				t.Fatalf("got %s want %s", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("automatic run missing")
		}
	}
	waitRPCIdle(t, d)
	for _, id := range []string{first.RequestID, second.RequestID} {
		r, e := l.Lookup(id)
		if e != nil || r.State != "completed" {
			t.Fatalf("missing terminal: %+v %v", r, e)
		}
	}
	select {
	case v := <-violations:
		t.Fatalf("predecessor not durable before %s", v)
	default:
	}
}
func TestAutomaticCancellationRequiresResumeAndReplayStillWorks(t *testing.T) {
	l, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	b := &automaticBackend{entered: make(chan string, 8), release: make(chan struct{})}
	s := app.New(b, app.Options{SessionID: "session", MaxIterations: 1})
	d := NewDispatcher(s, l)
	defer d.Close()
	call := func(id, method, params string) {
		t.Helper()
		r := d.Dispatch(context.Background(), rpcRequest(id, method, params))
		if r.Error != nil {
			t.Fatal(r.Error)
		}
	}
	call("h", "hello", `{}`)
	call("manual", "followup.enqueue", `{"text":"manual"}`)
	call("p1", "followup.start", `{}`)
	completedRequest(t, l)
	waitRPCIdle(t, d)
	<-b.entered
	call("parent", "prompt", `{"text":"parent"}`)
	<-b.entered
	call("q1", "followup.enqueue", `{"text":"first","auto":true}`)
	call("c", "cancel", `{}`)
	waitRPCIdle(t, d)
	call("q2", "followup.enqueue", `{"text":"second","auto":true}`)
	call("p1", "followup.start", `{}`) // Replay must work even with an automatic queue head.
	if len(s.QueuedInputs()) != 2 {
		t.Fatal("cancelled queue was consumed")
	}
	select {
	case got := <-b.entered:
		t.Fatalf("paused follow-up executed: %s", got)
	default:
	}
	call("resume", "followup.resume", `{}`)
	for _, want := range []string{"first", "second"} {
		select {
		case got := <-b.entered:
			if got != want {
				t.Fatal(got)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("resume did not continue")
		}
	}
	waitRPCIdle(t, d)
}
