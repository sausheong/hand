package agentio

import (
	"context"
	"encoding/json"

	"github.com/sausheong/harness/runtime"
)

// SystemPrompt is agcode's fixed identity prompt for Phase 1 (no
// per-project customization yet).
const SystemPrompt = `You are agcode, a terminal-based coding assistant. You can read, write, and edit files in the current workspace, run shell commands, and fetch or search the web. Track multi-step work with the todo tool. Be direct and concise: prefer making the requested change over describing what you would do.`

// MaxTurns caps the tool-use loop for a single agcode run.
const MaxTurns = 50

// BuildAgentSpec builds the single AgentSpec agcode uses for the whole
// process. hook is wired as Loop.Hooks.BeforeToolUse — callers pass the
// closure returned by NewApprovalHook.
func BuildAgentSpec(model, workspace string, hook func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error)) runtime.AgentSpec {
	return runtime.AgentSpec{
		ID:           "agcode",
		Name:         "agcode",
		Model:        model,
		Workspace:    workspace,
		SystemPrompt: SystemPrompt,
		MaxTurns:     MaxTurns,
		Loop: runtime.LoopConfig{
			Hooks: runtime.LifecycleHooks{
				BeforeToolUse: hook,
			},
		},
	}
}
