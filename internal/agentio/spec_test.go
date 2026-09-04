package agentio_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/runtime"
)

func TestBuildAgentSpec_SetsExpectedFields(t *testing.T) {
	hook := func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
		return runtime.HookDecision{Allow: true}, nil
	}

	spec := agentio.BuildAgentSpec("anthropic/claude-sonnet-5", "/tmp/work", hook)

	if spec.Model != "anthropic/claude-sonnet-5" {
		t.Errorf("Model = %q, want %q", spec.Model, "anthropic/claude-sonnet-5")
	}
	if spec.Workspace != "/tmp/work" {
		t.Errorf("Workspace = %q, want %q", spec.Workspace, "/tmp/work")
	}
	if spec.SystemPrompt == "" {
		t.Error("SystemPrompt is empty")
	}
	if spec.MaxTurns <= 0 {
		t.Errorf("MaxTurns = %d, want > 0", spec.MaxTurns)
	}
	if spec.Loop.Hooks.BeforeToolUse == nil {
		t.Error("Loop.Hooks.BeforeToolUse is nil, want the provided hook")
	}
}
