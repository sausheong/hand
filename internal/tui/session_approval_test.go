package tui

import (
	"context"
	"encoding/json"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/runtime"
	"strings"
	"testing"
)

func TestSessionAutoApprovalVisibleWhileIdleAndRunning(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	defer m.CloseApplication()
	if strings.Contains(m.statusLine(), "approvals off") {
		t.Fatal("normal mode mislabelled")
	}
	m.SetSessionAutoApproval(true)
	for _, running := range []bool{false, true} {
		m.running = running
		if !strings.Contains(m.statusLine(), "approvals off") {
			t.Fatal("missing approval mode indicator")
		}
	}
}

func TestPermissionCommandsChangeLivePolicyAndRestoreGrants(t *testing.T) {
	policy := &app.SessionApproval{}
	m := NewModel(nil, t.TempDir())
	defer m.CloseApplication()
	m.controller = &app.Controller{SessionApproval: policy}
	called := 0
	hook := policy.Wrap(func(_ context.Context, name string, _ json.RawMessage) (runtime.HookDecision, error) {
		called++
		return runtime.HookDecision{Allow: name == "saved-grant"}, nil
	})
	m.handleCommand("/permissions skip")
	decision, _ := hook(context.Background(), "write_file", nil)
	if !decision.Allow || called != 0 || !strings.Contains(m.statusLine(), "approvals off") {
		t.Fatal("skip command did not control real approval hook")
	}
	m.handleCommand("/permissions")
	if !strings.Contains(strings.Join(m.transcript, "\n"), "Approval mode: skip") {
		t.Fatal("inspection omitted current mode")
	}
	m.handleCommand("/permissions ask")
	decision, _ = hook(context.Background(), "write_file", nil)
	if decision.Allow || called != 1 || strings.Contains(m.statusLine(), "approvals off") {
		t.Fatal("ask command did not restore normal hook")
	}
	decision, _ = hook(context.Background(), "saved-grant", nil)
	if !decision.Allow {
		t.Fatal("ask revoked a saved permission")
	}
	m.running = true
	m.runPermissionCommand([]string{"skip"})
	if policy.Skipping() {
		t.Fatal("changed mode during active turn")
	}
	m.running = false
	m.handleCommand("/permissions skip extra")
	if policy.Skipping() {
		t.Fatal("invalid command changed policy")
	}
	policy.SetSkip(true) // Also represents a session launched with the CLI flag.
	m.handleCommand("/permissions ask")
	if policy.Skipping() {
		t.Fatal("ask did not override launch setting")
	}
}
