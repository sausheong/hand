//go:build darwin || linux

package rpc

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/protocol"
	"github.com/sausheong/harness/llm"
)

type rpcBackend struct {
	calls  atomic.Int32
	hang   bool
	joined chan struct{}
}

func (b *rpcBackend) StopReason() string { return "" }
func (b *rpcBackend) Run(ctx context.Context, _ string, _ []llm.ImageContent) (<-chan app.BackendEvent, error) {
	b.calls.Add(1)
	events := make(chan app.BackendEvent)
	go func() {
		defer close(events)
		defer close(b.joined)
		if b.hang {
			<-ctx.Done()
			return
		}
		for i := 0; i < 400; i++ {
			select {
			case events <- app.BackendEvent{Kind: "text", Text: "delta"}:
			case <-ctx.Done():
				return
			}
		}
		events <- app.BackendEvent{Done: true}
	}()
	return events, nil
}
func rpcRequest(id, method, params string) protocol.Request {
	return protocol.Request{Version: 1, ID: id, Method: method, Params: json.RawMessage(params)}
}
func completedRequest(t *testing.T, l *Ledger) RequestRecord {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		r, err := l.Lookup("p1")
		if err == nil && r.State == "completed" {
			return r
		}
		select {
		case <-deadline:
			t.Fatalf("request not completed: %+v %v", r, err)
		case <-time.After(time.Millisecond):
		}
	}
}
func TestDispatcherPromptReplayAndBoundedPoll(t *testing.T) {
	l, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	backend := &rpcBackend{joined: make(chan struct{})}
	d := NewDispatcher(app.New(backend, app.Options{SessionID: "session", MaxIterations: 1}), l)
	defer d.Close()
	if r := d.Dispatch(context.Background(), rpcRequest("s", "state", `{}`)); r.Error == nil {
		t.Fatal("negotiation bypass")
	}
	d.Dispatch(context.Background(), rpcRequest("h", "hello", `{}`))
	req := rpcRequest("p1", "prompt", `{"text":"hello"}`)
	if r := d.Dispatch(context.Background(), req); r.Error != nil {
		t.Fatal(r.Error)
	}
	complete := completedRequest(t, l)
	if r := d.Dispatch(context.Background(), req); r.Error != nil {
		t.Fatal(r.Error)
	}
	if backend.calls.Load() != 1 {
		t.Fatal("duplicate prompt executed")
	}
	var terminal protocol.Event
	if err = json.Unmarshal(complete.Result, &terminal); err != nil || terminal.Kind != "terminal" || terminal.RunID != complete.RunID {
		t.Fatalf("terminal %+v %v", terminal, err)
	}
	r := d.Dispatch(context.Background(), rpcRequest("poll", "events.poll", `{"after":0}`))
	var batch struct {
		Events []retainedEvent `json:"events"`
		Gap    bool            `json:"gap"`
	}
	if err = json.Unmarshal(r.Result, &batch); err != nil || !batch.Gap || len(batch.Events) > 16 {
		t.Fatalf("poll %s %v", r.Result, err)
	}
	d.mu.Lock()
	count, size := len(d.events), d.bytes
	d.mu.Unlock()
	if count > RetainedEvents || size > RetainedBytes {
		t.Fatal("retention exceeded")
	}
}
func TestDispatcherDisconnectCancelsAndPersistsTerminal(t *testing.T) {
	l, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	backend := &rpcBackend{hang: true, joined: make(chan struct{})}
	d := NewDispatcher(app.New(backend, app.Options{SessionID: "session", MaxIterations: 1}), l)
	d.Dispatch(context.Background(), rpcRequest("h", "hello", `{}`))
	if r := d.Dispatch(context.Background(), rpcRequest("p1", "prompt", `{"text":"hello"}`)); r.Error != nil {
		t.Fatal(r.Error)
	}
	// This scenario tests joining active work, not cancellation before launch.
	deadline := time.After(3 * time.Second)
	for backend.calls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("backend did not start")
		case <-time.After(time.Millisecond):
		}
	}

	d.Close()
	select {
	case <-backend.joined:
	default:
		t.Fatal("backend not joined")
	}
	completedRequest(t, l)
	if r := d.Dispatch(context.Background(), rpcRequest("s", "state", `{}`)); r.Error == nil {
		t.Fatal("dispatch after close")
	}
}
