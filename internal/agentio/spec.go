package agentio

import (
	"context"
	"encoding/json"

	"github.com/sausheong/harness/runtime"
)

// SystemPrompt is Hand's fixed identity prompt for Phase 1 (no
// per-project customization yet).
const SystemPrompt = `You are Hand, a terminal-based coding assistant. You can read, write, and edit files in the current workspace, run shell commands, and fetch or search the web. Track multi-step work with the todo tool. Be direct and concise: prefer making the requested change over describing what you would do.`

// BuildAgentSpec builds the single AgentSpec Hand uses for the whole
// process. hook is wired as Loop.Hooks.BeforeToolUse — callers pass the
// closure returned by NewApprovalHook. maxTurns caps the tool-use loop
// for a single run; callers resolve it (flag, config, or default) via
// internal/config.ResolveMaxTurns before calling this.
func BuildAgentSpec(model, workspace string, maxTurns int, hook func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error)) runtime.AgentSpec {
	return runtime.AgentSpec{
		ID:           "hand",
		Name:         "Hand",
		Model:        model,
		Workspace:    workspace,
		SystemPrompt: SystemPrompt,
		MaxTurns:     maxTurns,
		Loop: runtime.LoopConfig{
			Hooks: runtime.LifecycleHooks{
				BeforeToolUse: hook,
			},
		},
	}
}
