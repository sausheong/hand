// Package agentio wires harness's tool registry, agent spec, and
// approval gating for Hand.
package agentio

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sausheong/hand/internal/permissions"
	"github.com/sausheong/harness/runtime"
)

// Sender delivers a message to the TUI program. Declared with a plain
// `any` parameter (rather than bubbletea's tea.Msg) so this package has
// no dependency on the TUI framework; internal/tui provides the
// concrete adapter around *tea.Program.
type Sender interface {
	Send(msg any)
}

// Decision is the user's answer to an ApprovalRequest.
type Decision int

const (
	// DecisionDeny denies this call and does not persist anything.
	DecisionDeny Decision = iota
	// DecisionOnce allows this call only; the same tool will prompt
	// again next time.
	DecisionOnce
	// DecisionAlways allows this call and persists the tool name to the
	// permissions.Store so future calls to it never prompt again.
	DecisionAlways
)

// ApprovalRequest is sent to the TUI when a gated tool call needs a
// decision before it may proceed. Respond must receive exactly one
// value — the BeforeToolUse hook blocks reading it until it does.
type ApprovalRequest struct {
	Tool    string
	Input   json.RawMessage
	Preview string // human-readable preview (a diff, or the bash command); "" if none
	Respond chan Decision
}

// gatedTools names the tools that require approval before executing.
// read_file, web_fetch, web_search, and todo_write are never gated.
var gatedTools = map[string]bool{
	"write_file": true,
	"edit_file":  true,
	"bash":       true,
}

// mcpToolPrefix is harness's namespacing convention for adapter tools
// registered from a connected MCP server (runtime/builder.go:
// "mcp__<Name>__<tool>"). Pinned as a literal constant rather than
// derived from harness, so a harness upgrade that changes this
// convention fails this package's tests loudly instead of silently
// un-gating every MCP tool.
const mcpToolPrefix = "mcp__"

// mcpServerName extracts <Name> from a harness MCP adapter tool name of
// the form "mcp__<Name>__<tool>". isMCP is true for anything carrying
// the "mcp__" prefix, even a malformed name with no second "__" — that
// case returns an empty, unmatchable server string, which can never
// appear in a trustedServers map, so a malformed MCP tool name fails
// safe as gated rather than silently slipping through ungated.
func mcpServerName(toolName string) (server string, isMCP bool) {
	if !strings.HasPrefix(toolName, mcpToolPrefix) {
		return "", false
	}
	rest := strings.TrimPrefix(toolName, mcpToolPrefix)
	server, _, _ = strings.Cut(rest, "__")
	return server, true
}

// isGated reports whether name requires approval before executing. A
// built-in tool is gated iff it's in gatedTools. An MCP-provided tool
// (namespaced "mcp__<Name>__<tool>") is gated unless its server is
// marked trusted in trustedServers — an arbitrary MCP server can expose
// arbitrary mutating tools hand has never seen before, so the default is
// to gate everything it provides.
func isGated(name string, trustedServers map[string]bool) bool {
	if gatedTools[name] {
		return true
	}
	if server, isMCP := mcpServerName(name); isMCP {
		return !trustedServers[server]
	}
	return false
}

// NewApprovalHook returns a runtime.LifecycleHooks.BeforeToolUse
// closure. For a gated tool not already always-allowed by perms, it
// sends an ApprovalRequest (with a best-effort Preview built from the
// call's input against workspace) via sender and blocks until the
// request's Respond channel receives an answer (or the call's context
// is cancelled, which denies). Every other tool is allowed immediately
// with no prompt. perms may be nil, meaning no persistence — every
// gated call always prompts, matching Phase 1's behavior. trustedServers
// is the set of MCP server names (from config.Config.TrustedMCPServers)
// whose tools should skip gating entirely.
func NewApprovalHook(sender Sender, perms *permissions.Store, workspace string, trustedServers map[string]bool) func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
	return func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
		if !isGated(name, trustedServers) {
			return runtime.HookDecision{Allow: true}, nil
		}
		if perms != nil && perms.IsAlwaysAllowed(name) {
			return runtime.HookDecision{Allow: true}, nil
		}

		req := ApprovalRequest{Tool: name, Input: input, Preview: buildPreview(workspace, name, input), Respond: make(chan Decision, 1)}
		sender.Send(req)

		select {
		case decision := <-req.Respond:
			switch decision {
			case DecisionAlways:
				if perms != nil {
					if err := perms.SetAlwaysAllow(name); err != nil {
						// The user did approve this call; a persistence
						// failure shouldn't deny it, just mean the next
						// call prompts again too.
						return runtime.HookDecision{Allow: true}, nil
					}
				}
				return runtime.HookDecision{Allow: true}, nil
			case DecisionOnce:
				return runtime.HookDecision{Allow: true}, nil
			default:
				return runtime.HookDecision{Allow: false, Reason: fmt.Sprintf("user denied %s", name)}, nil
			}
		case <-ctx.Done():
			return runtime.HookDecision{Allow: false, Reason: "approval cancelled: " + ctx.Err().Error()}, nil
		}
	}
}

// NewOneShotApprovalHook returns a BeforeToolUse closure for
// non-interactive (-p) runs, where there is no TUI to prompt. A gated
// tool is allowed only if autoApprove is true (the --yes flag) or it is
// already always-allowed in perms (perms may be nil, meaning neither
// applies). Anything else is denied with a Reason explaining how to
// approve it: run hand interactively once and press 'a', or pass
// --yes. trustedServers is the set of MCP server names (from
// config.Config.TrustedMCPServers) whose tools should skip gating
// entirely.
func NewOneShotApprovalHook(perms *permissions.Store, autoApprove bool, trustedServers map[string]bool) func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
	return func(_ context.Context, name string, _ json.RawMessage) (runtime.HookDecision, error) {
		if !isGated(name, trustedServers) {
			return runtime.HookDecision{Allow: true}, nil
		}
		if autoApprove {
			return runtime.HookDecision{Allow: true}, nil
		}
		if perms != nil && perms.IsAlwaysAllowed(name) {
			return runtime.HookDecision{Allow: true}, nil
		}
		return runtime.HookDecision{
			Allow: false,
			Reason: fmt.Sprintf(
				"%s is not always-allowed for this project; run hand interactively once and press 'a' to approve it, or pass --yes to bypass approval for this one-shot run",
				name,
			),
		}, nil
	}
}
