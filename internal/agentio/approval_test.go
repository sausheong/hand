package agentio_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/sausheong/agcode/internal/agentio"
	"github.com/sausheong/harness/runtime"
)

type fakeSender struct {
	mu   sync.Mutex
	sent []any
}

func (f *fakeSender) Send(msg any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, msg)
}

func (f *fakeSender) first() (any, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sent) == 0 {
		return nil, false
	}
	return f.sent[0], true
}

func waitForRequest(t *testing.T, sender *fakeSender) agentio.ApprovalRequest {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		if msg, ok := sender.first(); ok {
			req, ok := msg.(agentio.ApprovalRequest)
			if !ok {
				t.Fatalf("sent message has type %T, want agentio.ApprovalRequest", msg)
			}
			return req
		}
		select {
		case <-deadline:
			t.Fatal("hook never sent an approval request")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestApprovalHook_UngatedToolsAllowedImmediately(t *testing.T) {
	for _, name := range []string{"read_file", "web_fetch", "web_search", "todo_write"} {
		t.Run(name, func(t *testing.T) {
			sender := &fakeSender{}
			hook := agentio.NewApprovalHook(sender)

			decision, err := hook(context.Background(), name, json.RawMessage(`{}`))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !decision.Allow {
				t.Fatalf("expected Allow=true for ungated tool %q, got %+v", name, decision)
			}
			if _, ok := sender.first(); ok {
				t.Fatalf("ungated tool %q should never prompt", name)
			}
		})
	}
}

func TestApprovalHook_GatedToolBlocksThenRespects(t *testing.T) {
	cases := []struct {
		tool   string
		answer bool
	}{
		{"write_file", true},
		{"write_file", false},
		{"edit_file", true},
		{"bash", false},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			sender := &fakeSender{}
			hook := agentio.NewApprovalHook(sender)

			resultCh := make(chan struct {
				decision runtime.HookDecision
				err      error
			}, 1)
			go func() {
				decision, err := hook(context.Background(), tc.tool, json.RawMessage(`{"x":1}`))
				resultCh <- struct {
					decision runtime.HookDecision
					err      error
				}{decision, err}
			}()

			req := waitForRequest(t, sender)
			if req.Tool != tc.tool {
				t.Fatalf("ApprovalRequest.Tool = %q, want %q", req.Tool, tc.tool)
			}

			select {
			case <-resultCh:
				t.Fatal("hook returned before the approval was answered")
			case <-time.After(20 * time.Millisecond):
			}

			req.Respond <- tc.answer

			select {
			case res := <-resultCh:
				if res.err != nil {
					t.Fatalf("unexpected error: %v", res.err)
				}
				if res.decision.Allow != tc.answer {
					t.Fatalf("Allow = %v, want %v", res.decision.Allow, tc.answer)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("hook did not return after approval was answered")
			}
		})
	}
}

func TestApprovalHook_ContextCancelDenies(t *testing.T) {
	sender := &fakeSender{}
	hook := agentio.NewApprovalHook(sender)

	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan error, 1)
	var allow bool
	go func() {
		decision, err := hook(ctx, "bash", json.RawMessage(`{}`))
		allow = decision.Allow
		resultCh <- err
	}()

	waitForRequest(t, sender)
	cancel()

	select {
	case err := <-resultCh:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if allow {
			t.Fatal("expected Allow=false after context cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("hook did not return after context cancellation")
	}
}
