package agentio

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tools/mcp"
)

// baseSystemPrompt is Hand's fixed identity prompt.
const baseSystemPrompt = `You are Hand, a terminal-based coding assistant. You can read, write, and edit files in the current workspace, run shell commands, and fetch or search the web. Track multi-step work with the todo tool. Be direct and concise: prefer making the requested change over describing what you would do.`

// SystemPrompt is baseSystemPrompt's fallback form for callers/tests that
// reference the fixed identity text directly; BuildSystemPrompt is what
// Hand actually uses, since it also appends any project instructions.
const SystemPrompt = baseSystemPrompt

// projectInstructionFiles are checked in order; the first one found
// wins. HAND.md is hand-specific and takes precedence over the more
// widely-adopted AGENTS.md convention, so a repo that already has an
// AGENTS.md for other tools works with hand too without duplication.
var projectInstructionFiles = []string{"HAND.md", "AGENTS.md"}

// BuildSystemPrompt returns hand's fixed identity plus, if present, one
// project instruction file's content appended underneath. Only one
// workspace-root file is ever read — no merging across multiple
// directories (parent dirs, $HOME).
func BuildSystemPrompt(workspace string) string {
	for _, name := range projectInstructionFiles {
		data, err := os.ReadFile(filepath.Join(workspace, name))
		if err != nil {
			continue
		}
		return baseSystemPrompt + "\n\n---\n\nProject instructions (" + name + "):\n\n" + string(data)
	}
	return baseSystemPrompt
}

// BuildAgentSpec builds the single AgentSpec Hand uses for the whole
// process. hook is wired as Loop.Hooks.BeforeToolUse — callers pass the
// closure returned by NewApprovalHook. maxTurns caps the tool-use loop
// for a single run; callers resolve it (flag, config, or default) via
// internal/config.ResolveMaxTurns before calling this. fallbackModel is
// the "provider/model" to retry against on a transient provider error;
// empty means no fallback — harness treats "" as "no fallback".
// mcpServers is passed straight through to AgentSpec.MCPServers for
// BuildRuntime to connect; nil/empty preserves today's zero-servers
// behavior unchanged.
func BuildAgentSpec(model, workspace string, maxTurns int, fallbackModel string, mcpServers []mcp.ServerConfig, hook func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error)) runtime.AgentSpec {
	return runtime.AgentSpec{
		ID:            "hand",
		Name:          "Hand",
		Model:         model,
		FallbackModel: fallbackModel,
		Workspace:     workspace,
		SystemPrompt:  BuildSystemPrompt(workspace),
		MaxTurns:      maxTurns,
		MCPServers:    mcpServers,
		Loop: runtime.LoopConfig{
			Hooks: runtime.LifecycleHooks{
				BeforeToolUse: hook,
			},
		},
	}
}
