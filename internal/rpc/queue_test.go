//go:build darwin || linux

package rpc

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/llm"
)

type queuedBackend struct {
	calls   atomic.Int32
	prompts chan string
}

func (*queuedBackend) StopReason() string { return "" }
func (b *queuedBackend) Run(_ context.Context, text string, _ []llm.ImageContent) (<-chan app.BackendEvent, error) {
	b.calls.Add(1)
	b.prompts <- text
	ch := make(chan app.BackendEvent, 1)
	ch <- app.BackendEvent{Done: true}
	close(ch)
	return ch, nil
}
func TestRPCQueueControlsAndFollowupExecution(t *testing.T) {
	l, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	backend := &queuedBackend{prompts: make(chan string, 4)}
	service := app.New(backend, app.Options{SessionID: "session", MaxIterations: 1})
	d := NewDispatcher(service, l)
	defer d.Close()
	call := func(id, method, params string) json.RawMessage {
		t.Helper()
		r := d.Dispatch(context.Background(), rpcRequest(id, method, params))
		if r.Error != nil {
			t.Fatal(r.Error)
		}
		return r.Result
	}
	call("hello", "hello", `{}`)
	var steering app.QueuedInput
	if err = json.Unmarshal(call("s", "steer", `{"text":"correct direction"}`), &steering); err != nil {
		t.Fatal(err)
	}
	if steering.Queue != app.SteeringQueue {
		t.Fatal("wrong steering queue")
	}
	raw, _ := json.Marshal(map[string]string{"id": steering.ID, "text": "revised direction"})
	call("edit", "queue.edit", string(raw))
	if service.QueuedInputs()[0].Text != "revised direction" {
		t.Fatal("queue edit not applied")
	}
	raw, _ = json.Marshal(map[string]string{"id": steering.ID})
	call("remove", "queue.remove", string(raw))
	call("f1", "followup.enqueue", `{"text":"first follow-up"}`)
	call("f2", "followup.enqueue", `{"text":"second follow-up"}`)
	call("p1", "followup.start", `{}`)
	completedRequest(t, l)
	call("p1", "followup.start", `{}`)
	if backend.calls.Load() != 1 {
		t.Fatal("duplicate follow-up start executed again")
	}
	select {
	case text := <-backend.prompts:
		if text != "first follow-up" {
			t.Fatal(text)
		}
	case <-time.After(time.Second):
		t.Fatal("follow-up not delivered")
	}
	inputs := service.QueuedInputs()
	if len(inputs) != 1 || inputs[0].Text != "second follow-up" {
		t.Fatalf("queue consumed twice: %+v", inputs)
	}
	call("list", "queue.list", `{"offset":0}`)
}
