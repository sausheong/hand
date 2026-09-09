package app

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/tool"
)

type extensionPermission struct {
	previous tool.PermissionChecker
	host     *ExtensionHost
}

func (p *extensionPermission) FilterToolDefs(defs []llm.ToolDef, agentID string) []llm.ToolDef {
	if p.previous != nil {
		return p.previous.FilterToolDefs(defs, agentID)
	}
	return defs
}
func (p *extensionPermission) Check(ctx context.Context, agentID, name string, input json.RawMessage) tool.Decision {
	if p.previous != nil {
		decision := p.previous.Check(ctx, agentID, name, input)
		if decision.Behavior != tool.DecisionAllow {
			return decision
		}
	}
	enabled, err := p.host.manager.CapabilityEnabled("policy.check")
	if err != nil {
		return tool.Decision{Behavior: tool.DecisionDeny, Reason: err.Error()}
	}
	if !enabled {
		return tool.Decision{Behavior: tool.DecisionAllow}
	}
	diagnostics, err := p.host.manager.CheckPolicy(context.WithValue(ctx, extensionPolicyContextKey{}, true), extensions.PolicyRequest{Action: "tool.execute", Resource: name, Input: append(json.RawMessage(nil), input...)}, true)
	for _, diagnostic := range diagnostics {
		slog.Warn("optional extension policy check failed", "extension", diagnostic.ID, "error", diagnostic.Error)
	}
	if err != nil {
		return tool.Decision{Behavior: tool.DecisionDeny, Reason: err.Error()}
	}
	return tool.Decision{Behavior: tool.DecisionAllow}
}
func (h *ExtensionHost) attachRuntimePolicy() {
	if h.controller.Rt != nil {
		h.controller.Rt.Permission = &extensionPermission{previous: h.controller.Rt.Permission, host: h}
	}
}
