package agentio_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/permissions"
)

func TestSkillOperationsRequireApproval(t *testing.T) {
	for _, action := range []string{"create", "patch", "replace", "remove", "unknown", ""} {
		t.Run(action, func(t *testing.T) {
			input, _ := json.Marshal(map[string]string{"action": action})
			decision, err := agentio.NewOneShotApprovalHook(nil, false, nil, nil)(context.Background(), "skill_manage", input)
			if err != nil || decision.Allow {
				t.Fatalf("mutation must require approval: %+v %v", decision, err)
			}
		})
	}
	for _, action := range []string{"list", "get"} {
		input, _ := json.Marshal(map[string]string{"action": action})
		decision, err := agentio.NewOneShotApprovalHook(nil, false, nil, nil)(context.Background(), "skill_manage", input)
		if err != nil || !decision.Allow {
			t.Fatalf("read action %s: %+v %v", action, decision, err)
		}
	}
}

func TestPersistentTodoAndUnknownToolsRequireApproval(t *testing.T) {
	for _, name := range []string{"todo_write", "future_tool"} {
		d, _ := agentio.NewOneShotApprovalHook(nil, false, nil, nil)(context.Background(), name, json.RawMessage(`{}`))
		if d.Allow {
			t.Fatalf("%s silently allowed", name)
		}
	}
}

func TestFailedGrantSaveDoesNotAllowNextCall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "parent")
	if err := os.WriteFile(path, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	store := permissions.NewEmptyStore(filepath.Join(path, "settings.json"))
	if err := store.SetAlwaysAllow("bash"); err == nil {
		t.Fatal("expected persistence failure")
	}
	if store.IsAlwaysAllowed("bash") {
		t.Fatal("failed save installed an in-memory grant")
	}
	d, _ := agentio.NewOneShotApprovalHook(store, false, nil, nil)(context.Background(), "bash", json.RawMessage(`{}`))
	if d.Allow {
		t.Fatal("subsequent invocation bypassed approval")
	}
}

type decidingSender struct {
	decision agentio.Decision
	prompts  int
	warnings int
}

func (s *decidingSender) Send(msg any) {
	switch v := msg.(type) {
	case agentio.ApprovalRequest:
		s.prompts++
		v.Respond <- s.decision
	case agentio.ApprovalWarning:
		s.warnings++
	}
}

func TestInteractiveSkillApprovalAndReadParity(t *testing.T) {
	for _, action := range []string{"create", "patch", "replace", "remove"} {
		s := &decidingSender{decision: agentio.DecisionDeny}
		input, _ := json.Marshal(map[string]string{"action": action})
		d, err := agentio.NewApprovalHook(s, nil, t.TempDir(), nil, nil)(context.Background(), "skill_manage", input)
		if err != nil || d.Allow || s.prompts != 1 {
			t.Fatalf("%s: %+v %v prompts=%d", action, d, err, s.prompts)
		}
		s.decision = agentio.DecisionOnce
		d, err = agentio.NewApprovalHook(s, nil, t.TempDir(), nil, nil)(context.Background(), "skill_manage", input)
		if err != nil || !d.Allow {
			t.Fatalf("explicit approval: %+v %v", d, err)
		}
	}
	for _, action := range []string{"list", "get"} {
		s := &decidingSender{decision: agentio.DecisionDeny}
		input, _ := json.Marshal(map[string]string{"action": action})
		d, err := agentio.NewApprovalHook(s, nil, t.TempDir(), nil, nil)(context.Background(), "skill_manage", input)
		if err != nil || !d.Allow || s.prompts != 0 {
			t.Fatalf("read action prompted: %s", action)
		}
	}
}

func TestInteractiveFailedSaveWarnsAndReprompts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "parent")
	if err := os.WriteFile(path, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	store := permissions.NewEmptyStore(filepath.Join(path, "settings.json"))
	s := &decidingSender{decision: agentio.DecisionAlways}
	hook := agentio.NewApprovalHook(s, store, t.TempDir(), nil, nil)
	d, err := hook(context.Background(), "bash", json.RawMessage(`{}`))
	if err != nil || !d.Allow || s.warnings != 1 {
		t.Fatalf("approved invocation/warning: %+v %v warnings=%d", d, err, s.warnings)
	}
	s.decision = agentio.DecisionDeny
	d, err = hook(context.Background(), "bash", json.RawMessage(`{}`))
	if err != nil || d.Allow || s.prompts != 2 {
		t.Fatalf("next invocation not gated: %+v %v prompts=%d", d, err, s.prompts)
	}
}
