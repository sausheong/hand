// Package agentio wires harness's tool registry, agent spec, and
// approval gating for agcode.
package agentio

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sausheong/harness/runtime"
)

// Sender delivers a message to the TUI program. Declared with a plain
// `any` parameter (rather than bubbletea's tea.Msg) so this package has
// no dependency on the TUI framework; internal/tui provides the
// concrete adapter around *tea.Program.
type Sender interface {
	Send(msg any)
}

// ApprovalRequest is sent to the TUI when a gated tool call needs a
// yes/no decision before it may proceed. Respond must receive exactly
// one value — the BeforeToolUse hook blocks reading it until it does.
type ApprovalRequest struct {
	Tool    string
	Input   json.RawMessage
	Respond chan bool
}

// gatedTools names the tools that require approval before executing.
// read_file, web_fetch, web_search, and todo_write are never gated.
var gatedTools = map[string]bool{
	"write_file": true,
	"edit_file":  true,
	"bash":       true,
}

// NewApprovalHook returns a runtime.LifecycleHooks.BeforeToolUse
// closure. For a gated tool it sends an ApprovalRequest via sender and
// blocks until the request's Respond channel receives an answer (or
// the call's context is cancelled, which denies). Every other tool is
// allowed immediately with no prompt.
func NewApprovalHook(sender Sender) func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
	return func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
		if !gatedTools[name] {
			return runtime.HookDecision{Allow: true}, nil
		}

		req := ApprovalRequest{Tool: name, Input: input, Respond: make(chan bool, 1)}
		sender.Send(req)

		select {
		case allow := <-req.Respond:
			if allow {
				return runtime.HookDecision{Allow: true}, nil
			}
			return runtime.HookDecision{Allow: false, Reason: fmt.Sprintf("user denied %s", name)}, nil
		case <-ctx.Done():
			return runtime.HookDecision{Allow: false, Reason: "approval cancelled: " + ctx.Err().Error()}, nil
		}
	}
}
