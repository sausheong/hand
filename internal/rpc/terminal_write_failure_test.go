//go:build darwin || linux

package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/llm"
)

type terminalFailureBackend struct {
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
}

func (*terminalFailureBackend) StopReason() string { return "" }
func (b *terminalFailureBackend) Run(ctx context.Context, _ string, _ []llm.ImageContent) (<-chan app.BackendEvent, error) {
	b.calls.Add(1)
	close(b.entered)
	events := make(chan app.BackendEvent, 1)
	go func() {
		defer close(events)
		select {
		case <-ctx.Done():
			return
		case <-b.release:
			events <- app.BackendEvent{Done: true}
		}
	}()
	return events, nil
}

func TestTerminalWriteFailurePausesAutomaticFollowups(t *testing.T) {
	path := ledgerPath(t)
	l, err := OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	b := &terminalFailureBackend{entered: make(chan struct{}), release: make(chan struct{})}
	s := app.New(b, app.Options{SessionID: "terminal-failure", MaxIterations: 1})
	d := NewDispatcher(s, l)
	defer d.Close()
	ctx := context.Background()
	call := func(id, method, params string) json.RawMessage {
		t.Helper()
		r := d.Dispatch(ctx, rpcRequest(id, method, params))
		if r.Error != nil {
			t.Fatal(r.Error)
		}
		return r.Result
	}
	call("hello", "hello", `{}`)
	call("prompt", "prompt", `{"text":"first"}`)
	select {
	case <-b.entered:
	case <-time.After(time.Second):
		t.Fatal("provider not started")
	}
	call("enqueue", "followup.enqueue", `{"text":"must stay queued","auto":true}`)
	beforeQueue := s.QueuedInputs()
	if len(beforeQueue) != 1 {
		t.Fatal("followup not queued")
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.file.Close(); err != nil {
		t.Fatal(err)
	}
	close(b.release)
	joined := make(chan struct{})
	go func() { d.wg.Wait(); close(joined) }()
	select {
	case <-joined:
	case <-time.After(3 * time.Second):
		t.Fatal("terminal consumer did not join")
	}
	if b.calls.Load() != 1 {
		t.Fatal("failed terminal persistence dispatched followup")
	}
	afterQueue := s.QueuedInputs()
	if len(afterQueue) != 1 || afterQueue[0] != beforeQueue[0] {
		t.Fatal("failed terminal persistence consumed pending work")
	}
	var state struct {
		Failure string `json:"failure"`
		Paused  bool   `json:"followup_paused"`
	}
	if err := json.Unmarshal(call("state", "state", `{}`), &state); err != nil {
		t.Fatal(err)
	}
	if !state.Paused || !strings.Contains(state.Failure, "terminal persistence failed") {
		t.Fatalf("failure not exposed: %+v", state)
	}
	d.mu.Lock()
	for _, event := range d.events {
		if event.Event.Kind == "terminal" {
			d.mu.Unlock()
			t.Fatal("undurable terminal exposed as completion")
		}
	}
	active := d.active
	d.mu.Unlock()
	if active != nil {
		t.Fatal("finished worker retained active ownership")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed completion changed journal bytes")
	}
	if r := d.Dispatch(ctx, rpcRequest("resume", "followup.resume", `{}`)); r.Error == nil {
		t.Fatal("resume accepted with poisoned storage")
	}
	if b.calls.Load() != 1 {
		t.Fatal("resume executed despite journal failure")
	}
	d.Close()
	l.Close()
	reopened, err := OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	record, execute, err := reopened.Begin(rpcRequest("prompt", "prompt", `{"text":"first"}`), "terminal-failure")
	if err != nil || execute || record.State != "uncertain" || len(record.Result) != 0 {
		t.Fatalf("restart claimed completion or replayed: %+v %t %v", record, execute, err)
	}
}
