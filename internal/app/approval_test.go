package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/agentio"
)

func TestApplicationApprovalLifetime(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "decision", true: "cancel"}[cancel], func(t *testing.T) {
			hook := NewApprovalHook(nil, t.TempDir(), nil, nil)
			allowed := make(chan bool, 1)
			backend := &backendFixture{run: func(ctx context.Context, _ string) (<-chan BackendEvent, error) {
				ch := make(chan BackendEvent, 1)
				go func() {
					d, err := hook(ctx, "bash", []byte(`{"command":"echo approved"}`))
					allowed <- d.Allow
					ch <- BackendEvent{Done: true, Err: err}
					close(ch)
				}()
				return ch, nil
			}}
			s := New(backend, Options{SessionID: "session", MaxIterations: 1})
			stream, err := s.Start(context.Background(), "work", nil)
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			var request Event
			timer := time.After(2 * time.Second)
			for request.Kind != "approval_required" {
				select {
				case request = <-stream.Events:
				case <-timer:
					t.Fatal("no approval request")
				}
			}
			if request.ApprovalID == "" || request.State != AwaitingApproval || request.Details.ToolName != "bash" || s.Snapshot().State != AwaitingApproval {
				t.Fatal(request)
			}
			identity := stream.Identity()
			stale := identity
			stale.Generation++
			if !errors.Is(s.RespondApproval(stale, request.ApprovalID, agentio.DecisionAlways), ErrApprovalExpired) {
				t.Fatal("stale identity accepted")
			}
			if s.RespondApproval(identity, request.ApprovalID, agentio.Decision(999)) == nil {
				t.Fatal("invalid decision accepted")
			}
			if cancel {
				stream.Cancel()
				if !errors.Is(s.RespondApproval(identity, request.ApprovalID, agentio.DecisionAlways), ErrApprovalExpired) {
					t.Fatal("cancelled approval accepted")
				}
			} else {
				if err := s.RespondApproval(identity, request.ApprovalID, agentio.DecisionOnce); err != nil {
					t.Fatal(err)
				}
				if !errors.Is(s.RespondApproval(identity, request.ApprovalID, agentio.DecisionOnce), ErrApprovalExpired) {
					t.Fatal("duplicate decision accepted")
				}
			}
			for event := range stream.Events {
				if event.Kind == "approval_resolved" && event.State != Running {
					t.Fatal("resolved approval retained waiting state", event)
				}
			}
			outcome, err := stream.Wait()
			if err != nil {
				t.Fatal(err)
			}
			if <-allowed == cancel {
				t.Fatal("wrong tool decision")
			}
			expected := agentio.Completed
			if cancel {
				expected = agentio.Cancelled
			}
			if outcome.Status != expected || s.Snapshot().State != Idle {
				t.Fatal(outcome)
			}
			if !errors.Is(s.RespondApproval(identity, request.ApprovalID, agentio.DecisionAlways), ErrApprovalExpired) {
				t.Fatal("completed approval accepted")
			}
			s.mu.Lock()
			remaining := len(s.approvals)
			s.mu.Unlock()
			if remaining != 0 {
				t.Fatal("pending approval leaked")
			}
		})
	}
}

func TestApprovalRequiresApplicationContext(t *testing.T) {
	hook := NewApprovalHook(nil, t.TempDir(), nil, nil)
	decision, err := hook(context.Background(), "bash", []byte(`{}`))
	if err == nil || decision.Allow {
		t.Fatal("approval outside owned run")
	}
}
