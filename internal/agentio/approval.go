// Package agentio wires harness's tool registry, agent spec, and
// approval gating for Hand.
package agentio

import (
	"context"
	"encoding/json"
	"fmt"

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

// NewApprovalHook returns a runtime.LifecycleHooks.BeforeToolUse
// closure. For a gated tool not already always-allowed by perms, it
// sends an ApprovalRequest (with a best-effort Preview built from the
// call's input against workspace) via sender and blocks until the
// request's Respond channel receives an answer (or the call's context
// is cancelled, which denies). Every other tool is allowed immediately
// with no prompt. perms may be nil, meaning no persistence — every
// gated call always prompts, matching Phase 1's behavior.
func NewApprovalHook(sender Sender, perms *permissions.Store, workspace string) func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
	return func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
		if !gatedTools[name] {
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
// --yes.
func NewOneShotApprovalHook(perms *permissions.Store, autoApprove bool) func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
	return func(_ context.Context, name string, _ json.RawMessage) (runtime.HookDecision, error) {
		if !gatedTools[name] {
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
