package agentio_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tools/mcp"
)

func TestBuildAgentSpec_SetsExpectedFields(t *testing.T) {
	hook := func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
		return runtime.HookDecision{Allow: true}, nil
	}
	hooks := runtime.LifecycleHooks{BeforeToolUse: hook}

	spec := agentio.BuildAgentSpec("anthropic/claude-sonnet-5", "/tmp/work", 42, "anthropic/claude-haiku-4-5", nil, hooks)

	if spec.Model != "anthropic/claude-sonnet-5" {
		t.Errorf("Model = %q, want %q", spec.Model, "anthropic/claude-sonnet-5")
	}
	if spec.Workspace != "/tmp/work" {
		t.Errorf("Workspace = %q, want %q", spec.Workspace, "/tmp/work")
	}
	if spec.SystemPrompt == "" {
		t.Error("SystemPrompt is empty")
	}
	if spec.MaxTurns != 42 {
		t.Errorf("MaxTurns = %d, want 42", spec.MaxTurns)
	}
	if spec.FallbackModel != "anthropic/claude-haiku-4-5" {
		t.Errorf("FallbackModel = %q, want %q", spec.FallbackModel, "anthropic/claude-haiku-4-5")
	}
	if spec.Loop.Hooks.BeforeToolUse == nil {
		t.Error("Loop.Hooks.BeforeToolUse is nil, want the provided hook")
	}
}

func TestBuildAgentSpec_EmptyFallbackModel(t *testing.T) {
	hook := func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
		return runtime.HookDecision{Allow: true}, nil
	}
	hooks := runtime.LifecycleHooks{BeforeToolUse: hook}

	spec := agentio.BuildAgentSpec("anthropic/claude-sonnet-5", "/tmp/work", 42, "", nil, hooks)

	if spec.FallbackModel != "" {
		t.Errorf("FallbackModel = %q, want empty", spec.FallbackModel)
	}
}

func TestBuildAgentSpec_SetsMCPServers(t *testing.T) {
	hook := func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
		return runtime.HookDecision{Allow: true}, nil
	}
	hooks := runtime.LifecycleHooks{BeforeToolUse: hook}
	servers := []mcp.ServerConfig{{Name: "github", Command: "npx"}}

	spec := agentio.BuildAgentSpec("anthropic/claude-sonnet-5", "/tmp/work", 42, "", servers, hooks)

	if len(spec.MCPServers) != 1 || spec.MCPServers[0].Name != "github" {
		t.Errorf("MCPServers = %+v, want %+v", spec.MCPServers, servers)
	}
}

func TestBuildAgentSpec_NilMCPServers(t *testing.T) {
	hook := func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
		return runtime.HookDecision{Allow: true}, nil
	}
	hooks := runtime.LifecycleHooks{BeforeToolUse: hook}

	spec := agentio.BuildAgentSpec("anthropic/claude-sonnet-5", "/tmp/work", 42, "", nil, hooks)

	if len(spec.MCPServers) != 0 {
		t.Errorf("MCPServers = %+v, want empty", spec.MCPServers)
	}
}

func TestBuildSystemPrompt_NoFileReturnsBaseOnly(t *testing.T) {
	got := agentio.BuildSystemPrompt(t.TempDir())

	if got != agentio.SystemPrompt {
		t.Errorf("BuildSystemPrompt = %q, want the base prompt unchanged", got)
	}
}

func TestBuildSystemPrompt_HandMdAppended(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "HAND.md"), []byte("use tabs, not spaces"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := agentio.BuildSystemPrompt(dir)

	if !strings.HasPrefix(got, agentio.SystemPrompt) {
		t.Errorf("BuildSystemPrompt = %q, want it to start with the base prompt", got)
	}
	if !strings.Contains(got, "use tabs, not spaces") {
		t.Errorf("BuildSystemPrompt = %q, want it to contain the HAND.md content", got)
	}
	if !strings.Contains(got, "HAND.md") {
		t.Errorf("BuildSystemPrompt = %q, want it to name the source file", got)
	}
}

func TestBuildSystemPrompt_AgentsMdAppendedWhenNoHandMd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("follow the style guide"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := agentio.BuildSystemPrompt(dir)

	if !strings.Contains(got, "follow the style guide") {
		t.Errorf("BuildSystemPrompt = %q, want it to contain the AGENTS.md content", got)
	}
	if !strings.Contains(got, "AGENTS.md") {
		t.Errorf("BuildSystemPrompt = %q, want it to name the source file", got)
	}
}

func TestBuildSystemPrompt_HandMdWinsOverAgentsMd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "HAND.md"), []byte("hand instructions"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("agents instructions"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := agentio.BuildSystemPrompt(dir)

	if !strings.Contains(got, "hand instructions") {
		t.Errorf("BuildSystemPrompt = %q, want HAND.md content present", got)
	}
	if strings.Contains(got, "agents instructions") {
		t.Errorf("BuildSystemPrompt = %q, want AGENTS.md content absent when HAND.md is present", got)
	}
}
