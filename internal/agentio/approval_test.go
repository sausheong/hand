package agentio_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/permissions"
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
			hook := agentio.NewApprovalHook(sender, nil, "")

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
		tool      string
		decision  agentio.Decision
		wantAllow bool
	}{
		{"write_file", agentio.DecisionOnce, true},
		{"write_file", agentio.DecisionDeny, false},
		{"edit_file", agentio.DecisionOnce, true},
		{"bash", agentio.DecisionDeny, false},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			sender := &fakeSender{}
			hook := agentio.NewApprovalHook(sender, nil, "")

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

			req.Respond <- tc.decision

			select {
			case res := <-resultCh:
				if res.err != nil {
					t.Fatalf("unexpected error: %v", res.err)
				}
				if res.decision.Allow != tc.wantAllow {
					t.Fatalf("Allow = %v, want %v", res.decision.Allow, tc.wantAllow)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("hook did not return after approval was answered")
			}
		})
	}
}

func TestApprovalHook_ContextCancelDenies(t *testing.T) {
	sender := &fakeSender{}
	hook := agentio.NewApprovalHook(sender, nil, "")

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

func TestApprovalHook_RequestIncludesPreview(t *testing.T) {
	sender := &fakeSender{}
	hook := agentio.NewApprovalHook(sender, nil, "/tmp/does-not-matter")

	go func() {
		_, _ = hook(context.Background(), "bash", json.RawMessage(`{"command":"echo hi"}`))
	}()

	req := waitForRequest(t, sender)
	if req.Preview != "$ echo hi" {
		t.Fatalf("Preview = %q, want %q", req.Preview, "$ echo hi")
	}
	req.Respond <- agentio.DecisionDeny
}

func TestApprovalHook_AlreadyAlwaysAllowedSkipsPrompt(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".hand", "settings.json")
	perms, err := permissions.NewStore(path)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	if err := perms.SetAlwaysAllow("bash"); err != nil {
		t.Fatalf("SetAlwaysAllow returned error: %v", err)
	}

	sender := &fakeSender{}
	hook := agentio.NewApprovalHook(sender, perms, "")

	decision, err := hook(context.Background(), "bash", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Allow {
		t.Fatalf("expected Allow=true for an already-always-allowed tool, got %+v", decision)
	}
	if _, ok := sender.first(); ok {
		t.Fatal("an already-always-allowed tool should never prompt")
	}
}

func TestApprovalHook_DecisionAlwaysPersistsBeforeReturning(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".hand", "settings.json")
	perms, err := permissions.NewStore(path)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}

	sender := &fakeSender{}
	hook := agentio.NewApprovalHook(sender, perms, "")

	resultCh := make(chan runtime.HookDecision, 1)
	go func() {
		decision, err := hook(context.Background(), "write_file", json.RawMessage(`{}`))
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		resultCh <- decision
	}()

	req := waitForRequest(t, sender)
	req.Respond <- agentio.DecisionAlways

	select {
	case decision := <-resultCh:
		if !decision.Allow {
			t.Fatalf("expected Allow=true after DecisionAlways, got %+v", decision)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("hook did not return after DecisionAlways")
	}

	if !perms.IsAlwaysAllowed("write_file") {
		t.Fatal("expected write_file to be always-allowed in the Store after DecisionAlways")
	}

	onDisk, err := permissions.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !onDisk.IsAlwaysAllowed("write_file") {
		t.Fatal("expected write_file to be persisted to disk after DecisionAlways")
	}
}

func TestOneShotApprovalHook_UngatedToolsAlwaysAllowed(t *testing.T) {
	hook := agentio.NewOneShotApprovalHook(nil, false)
	decision, err := hook(context.Background(), "read_file", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Allow {
		t.Fatalf("expected Allow=true for an ungated tool, got %+v", decision)
	}
}

func TestOneShotApprovalHook_GatedDeniedByDefault(t *testing.T) {
	hook := agentio.NewOneShotApprovalHook(nil, false)
	decision, err := hook(context.Background(), "bash", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Allow {
		t.Fatal("expected Allow=false for a gated tool with no allowlist and no autoApprove")
	}
	if decision.Reason == "" {
		t.Fatal("expected a Reason explaining how to approve the tool")
	}
}

func TestOneShotApprovalHook_GatedAllowedWhenAlwaysAllowed(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".hand", "settings.json")
	perms, err := permissions.NewStore(path)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	if err := perms.SetAlwaysAllow("bash"); err != nil {
		t.Fatalf("SetAlwaysAllow returned error: %v", err)
	}

	hook := agentio.NewOneShotApprovalHook(perms, false)
	decision, err := hook(context.Background(), "bash", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Allow {
		t.Fatalf("expected Allow=true for an already-always-allowed tool, got %+v", decision)
	}
}

func TestOneShotApprovalHook_GatedAllowedWithAutoApprove(t *testing.T) {
	hook := agentio.NewOneShotApprovalHook(nil, true)
	decision, err := hook(context.Background(), "write_file", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Allow {
		t.Fatalf("expected Allow=true with autoApprove, got %+v", decision)
	}
}
