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

// BuildSystemPrompt returns hand's fixed identity plus, if present, one
// project instruction file's content appended underneath. Only one
// workspace-root file is ever read — no merging across multiple
// directories (parent dirs, $HOME).
//
// The active model is deliberately NOT named here — see
// ModelIdentityHint's doc comment for why that line has to live in the
// per-turn dynamic prompt instead of this cached static one.
func BuildSystemPrompt(workspace string) string {
	prompt := baseSystemPrompt
	for _, name := range projectInstructionFiles {
		data, err := os.ReadFile(filepath.Join(workspace, name))
		if err != nil {
			continue
		}
		return prompt + "\n\n---\n\nProject instructions (" + name + "):\n\n" + string(data)
	}
	return prompt
}

// ModelIdentityHint returns the per-turn (uncached) text naming the
// active model, for Runtime.DynamicIdentityHint — set once at startup and
// again on every /model switch (see tui.Controller.SwitchModel).
//
// This can't just be baked into BuildSystemPrompt's cached static text:
// an LLM's own self-knowledge about which model it's running as is
// unreliable to begin with — especially behind an aggregator like
// OpenRouter or a proxy like LiteLLM, where the model actually serving
// the request may not match whatever the underlying weights "believe"
// about themselves from training — and once the conversation has a few
// turns in it, a model tends to just repeat whatever it already told the
// user earlier in that same conversation, outweighing a fact it only
// read once near the top of a large cached block. Resending the fact
// fresh, every turn, and explicitly telling it to override any earlier
// self-statement, gives it the same recency the model grants its own
// last answer.
func ModelIdentityHint(model string) string {
	if model == "" {
		return ""
	}
	return "You are running as the model \"" + model + "\" — if asked which model you are, answer with this exact string, even if you stated a different model earlier in this conversation (the model was switched)."
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
		SystemPrompt:  BuildSystemPrompt(workspace),
		MaxTurns:      maxTurns,
		MCPServers:    mcpServers,
		Loop: runtime.LoopConfig{
			Hooks: hooks,
		},
	}
}
