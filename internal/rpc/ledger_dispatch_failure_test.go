//go:build darwin || linux

package rpc

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/app"
)

func TestFailedJournalPreventsNewRPCWork(t *testing.T) {
	for _, method := range []string{"prompt", "followup.start", "followup.enqueue", "steer"} {
		t.Run(method, func(t *testing.T) {
			l, err := OpenLedger(ledgerPath(t))
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			b := &queuedBackend{prompts: make(chan string, 4)}
			s := app.New(b, app.Options{SessionID: "journal", MaxIterations: 1})
			d := NewDispatcher(s, l)
			defer d.Close()
			ctx := context.Background()
			if r := d.Dispatch(ctx, rpcRequest("h", "hello", `{}`)); r.Error != nil {
				t.Fatal(r.Error)
			}
			if _, err := s.EnqueueInput(app.FollowupQueue, "existing"); err != nil {
				t.Fatal(err)
			}
			before := s.QueuedInputs()
			if err := l.file.Close(); err != nil {
				t.Fatal(err)
			}
			params := `{"text":"new work"}`
			if method == "followup.start" {
				params = `{}`
			}
			for retry := 0; retry < 2; retry++ {
				r := d.Dispatch(ctx, rpcRequest("failed", method, params))
				if r.Error == nil || len(r.Result) != 0 {
					t.Fatal("failed journal returned success")
				}
				if b.calls.Load() != 0 || !reflect.DeepEqual(before, s.QueuedInputs()) {
					t.Fatal("failed journal permitted side effects")
				}
			}
		})
	}
}

func TestJournalFailureCannotPreventEmergencyCancellation(t *testing.T) {
	l, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	b := &rpcBackend{hang: true, joined: make(chan struct{})}
	d := NewDispatcher(app.New(b, app.Options{SessionID: "journal", MaxIterations: 1}), l)
	defer d.Close()
	ctx := context.Background()
	if r := d.Dispatch(ctx, rpcRequest("h", "hello", `{}`)); r.Error != nil {
		t.Fatal(r.Error)
	}
	if r := d.Dispatch(ctx, rpcRequest("p", "prompt", `{"text":"wait"}`)); r.Error != nil {
		t.Fatal(r.Error)
	}
	// A healthy request-ID conflict must not become an emergency cancel.
	if r := d.Dispatch(ctx, rpcRequest("p", "cancel", `{}`)); r.Error == nil {
		t.Fatal("conflicting request ID accepted")
	}
	select {
	case <-b.joined:
		t.Fatal("request conflict cancelled active work")
	case <-time.After(20 * time.Millisecond):
	}
	if err := l.file.Close(); err != nil {
		t.Fatal(err)
	}
	if r := d.Dispatch(ctx, rpcRequest("bad-stop", "cancel", `{"unexpected":true}`)); r.Error == nil {
		t.Fatal("malformed emergency cancellation accepted")
	}
	select {
	case <-b.joined:
		t.Fatal("malformed cancellation stopped active work")
	case <-time.After(20 * time.Millisecond):
	}
	r := d.Dispatch(ctx, rpcRequest("stop", "cancel", `{}`))
	// A cancellation signal is still necessary, but success cannot be claimed
	// when its receipt and the run's terminal outcome cannot be persisted.
	if r.Error == nil || r.Error.Code != "ledger_failure" || len(r.Result) != 0 {
		t.Fatal("unpersisted cancellation claimed success")
	}
	d.mu.Lock()
	paused := d.autoPaused
	d.mu.Unlock()
	if !paused {
		t.Fatal("journal failure did not pause automatic followups")
	}
	select {
	case <-b.joined:
	case <-time.After(time.Second):
		t.Fatal("journal failure prevented provider cancellation")
	}
}
