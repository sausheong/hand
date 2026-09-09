//go:build darwin || linux

package rpc

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/protocol"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
)

type approvalBackend struct {
	hook    func(context.Context, string, json.RawMessage) (runtime.HookDecision, error)
	allowed chan bool
}

func (b *approvalBackend) StopReason() string { return "" }
func (b *approvalBackend) Run(ctx context.Context, _ string, _ []llm.ImageContent) (<-chan app.BackendEvent, error) {
	out := make(chan app.BackendEvent, 1)
	go func() {
		defer close(out)
		decision, err := b.hook(ctx, "bash", json.RawMessage(`{"command":"echo approval-test"}`))
		b.allowed <- decision.Allow
		out <- app.BackendEvent{Done: true, Err: err}
	}()
	return out, nil
}
func pendingApproval(t *testing.T, d *Dispatcher) protocol.Event {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		response := d.Dispatch(context.Background(), rpcRequest("pending", "approval.pending", `{}`))
		if response.Error != nil {
			t.Fatal(response.Error)
		}
		var result struct {
			Approvals []protocol.Event `json:"approvals"`
		}
		if err := json.Unmarshal(response.Result, &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Approvals) > 0 {
			return result.Approvals[0]
		}
		select {
		case <-deadline:
			t.Fatal("approval not published")
		case <-time.After(time.Millisecond):
		}
	}
}
func TestRPCApprovalDecisionIdentityAndCancellation(t *testing.T) {
	for _, decision := range []string{"once", "deny", "cancel"} {
		t.Run(decision, func(t *testing.T) {
			l, err := OpenLedger(ledgerPath(t))
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			backend := &approvalBackend{hook: app.NewApprovalHook(nil, t.TempDir(), nil, nil), allowed: make(chan bool, 1)}
			d := NewDispatcher(app.New(backend, app.Options{SessionID: "session", MaxIterations: 1}), l)
			defer d.Close()
			d.Dispatch(context.Background(), rpcRequest("h", "hello", `{}`))
			if r := d.Dispatch(context.Background(), rpcRequest("p1", "prompt", `{"text":"execute"}`)); r.Error != nil {
				t.Fatal(r.Error)
			}
			event := pendingApproval(t, d)
			var payload struct {
				ID string `json:"approval_id"`
			}
			if err = json.Unmarshal(event.Payload, &payload); err != nil || payload.ID == "" {
				t.Fatal("approval ID missing")
			}
			answerID := 0
			respond := func(run, choice string) protocol.Response {
				answerID++
				p, _ := json.Marshal(map[string]string{"run_id": run, "approval_id": payload.ID, "decision": choice})
				return d.Dispatch(context.Background(), rpcRequest("answer-"+strconv.Itoa(answerID), "approval.respond", string(p)))
			}
			if r := respond("wrong-run", "once"); r.Error == nil {
				t.Fatal("wrong run authorised")
			}
			if r := respond(event.RunID, "invalid"); r.Error == nil {
				t.Fatal("invalid decision accepted")
			}
			if decision == "cancel" {
				d.Dispatch(context.Background(), rpcRequest("cancel", "cancel", `{}`))
				if r := respond(event.RunID, "once"); r.Error == nil {
					t.Fatal("cancelled approval authorised")
				}
			} else {
				if r := respond(event.RunID, decision); r.Error != nil {
					t.Fatal(r.Error)
				}
			}
			select {
			case allowed := <-backend.allowed:
				if allowed != (decision == "once") {
					t.Fatalf("allowed=%t for %s", allowed, decision)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("approval did not resolve")
			}
			completedRequest(t, l)
			d.Close()
			if r := respond(event.RunID, "once"); r.Error == nil {
				t.Fatal("stale approval reused")
			}
		})
	}
}
