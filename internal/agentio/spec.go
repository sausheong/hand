package agentio

import (
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

// BuildSystemPrompt returns hand's fixed identity, a line naming the
// active model (when model is non-empty), plus, if present, one project
// instruction file's content appended underneath. Only one
// workspace-root file is ever read — no merging across multiple
// directories (parent dirs, $HOME).
//
// The model line exists because an LLM's own self-knowledge about which
// model it is running as is unreliable — especially behind an
// aggregator like OpenRouter or a proxy like LiteLLM, where the model
// actually serving the request may not match whatever the underlying
// weights "believe" about themselves from training. Stating it
// explicitly, as an instruction for how to answer rather than just
// background info, is the only reliable fix.
func BuildSystemPrompt(workspace, model string) string {
	prompt := baseSystemPrompt
	if model != "" {
		prompt += "\n\nYou are running as the model \"" + model + "\" — if asked which model you are, answer with this exact string."
	}
	for _, name := range projectInstructionFiles {
		data, err := os.ReadFile(filepath.Join(workspace, name))
		if err != nil {
			continue
		}
		return prompt + "\n\n---\n\nProject instructions (" + name + "):\n\n" + string(data)
	}
	return prompt
}

// BuildAgentSpec builds the single AgentSpec Hand uses for the whole
// process. hooks is wired straight to Loop.Hooks — callers pass the
// result of agentio.BuildLifecycleHooks, which composes config.json's
// hooks list with the approval-prompt hook (NewApprovalHook /
// NewOneShotApprovalHook). maxTurns caps the tool-use loop for a single
// run; callers resolve it (flag, config, or default) via
// internal/config.ResolveMaxTurns before calling this. fallbackModel is
// the "provider/model" to retry against on a transient provider error;
// empty means no fallback — harness treats "" as "no fallback".
// mcpServers is passed straight through to AgentSpec.MCPServers for
// BuildRuntime to connect; nil/empty preserves today's zero-servers
// behavior unchanged.
func BuildAgentSpec(model, workspace string, maxTurns int, fallbackModel string, mcpServers []mcp.ServerConfig, hooks runtime.LifecycleHooks) runtime.AgentSpec {
	return runtime.AgentSpec{
		ID:            "hand",
		Name:          "Hand",
		Model:         model,
		FallbackModel: fallbackModel,
		Workspace:     workspace,
		SystemPrompt:  BuildSystemPrompt(workspace, model),
		MaxTurns:      maxTurns,
		MCPServers:    mcpServers,
		Loop: runtime.LoopConfig{
			Hooks: hooks,
		},
	}
}
