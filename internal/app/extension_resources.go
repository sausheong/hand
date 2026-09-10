package app

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/harness/tool"
)

type extensionPolicyContextKey struct{}

// File operations reuse the registered tool, hook and permission chain. A policy
// observer cannot recursively request host resources while evaluating policy.
func (h *ExtensionHost) readExtensionFile(ctx context.Context, spec extensions.Specification, params json.RawMessage) (json.RawMessage, *protocol.Error) {
	return h.extensionFileResource(ctx, spec, "file.read", params)
}

func (h *ExtensionHost) extensionFileResource(ctx context.Context, spec extensions.Specification, method string, params json.RawMessage) (json.RawMessage, *protocol.Error) {
	return h.extensionToolResource(ctx, spec, method, params)
}

func (h *ExtensionHost) extensionToolResource(ctx context.Context, spec extensions.Specification, method string, params json.RawMessage) (response json.RawMessage, failure *protocol.Error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	mutationStarted := false
	defer func() {
		if recover() != nil {
			response = nil
			failure = &protocol.Error{Code: "resource_failed", Message: "Host resource operation failed"}
			if mutationStarted {
				failure = &protocol.Error{Code: "mutation_outcome_unknown", Message: "Operation may have completed; inspect its effects before retrying"}
			}
		}
	}()
	fail := func(code, message string) (json.RawMessage, *protocol.Error) {
		if mutationStarted {
			return nil, &protocol.Error{Code: "mutation_outcome_unknown", Message: "Operation may have completed; inspect its effects before retrying"}
		}
		return nil, &protocol.Error{Code: code, Message: message}
	}
	if ctx.Value(extensionPolicyContextKey{}) != nil || slices.Contains(spec.Capabilities, "policy.check") {
		return fail("capability_denied", "Policy extensions cannot request host resources recursively")
	}
	var raw json.RawMessage
	var path string
	toolName := "read_file"
	switch method {
	case "file.read":
		var input struct {
			Path string `json:"path"`
		}
		if err := protocol.DecodePayload(params, &input); err != nil {
			return fail("invalid_input", "Invalid file.read input")
		}
		path = input.Path
		raw, _ = json.Marshal(input)
	case "file.write":
		toolName = "write_file"
		var input struct {
			Path    string  `json:"path"`
			Content *string `json:"content"`
			Digest  string  `json:"instruction_digest,omitempty"`
		}
		if err := protocol.DecodePayload(params, &input); err != nil || input.Content == nil || len(*input.Content) > 64<<10 || len(input.Digest) > 64 {
			return fail("invalid_input", "file.write requires content within 64 KiB and an optional instruction digest")
		}
		path = input.Path
		raw, _ = json.Marshal(input)
	case "network.fetch":
		toolName = "web_fetch"
		var input struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers,omitempty"`
		}
		if err := protocol.DecodePayload(params, &input); err != nil || len(input.URL) > 4096 || len(input.Headers) > 32 {
			return fail("invalid_input", "Invalid network.fetch input")
		}
		u, err := url.Parse(input.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
			return fail("invalid_input", "An HTTP or HTTPS URL without embedded credentials is required")
		}
		total := 0
		for name, value := range input.Headers {
			total += len(name) + len(value)
			if name == "" || strings.ContainsAny(name+value, "\r\n\x00") {
				return fail("invalid_input", "Invalid request header")
			}
		}
		if total > 8192 {
			return fail("invalid_input", "Request headers exceed 8 KiB")
		}
		raw, _ = json.Marshal(input)
	case "process.run":
		toolName = "bash"
		var input struct {
			Command string `json:"command"`
			Timeout int    `json:"timeout,omitempty"`
		}
		if err := protocol.DecodePayload(params, &input); err != nil || strings.TrimSpace(input.Command) == "" || len(input.Command) > 16<<10 || strings.ContainsRune(input.Command, 0) || input.Timeout < 0 || input.Timeout > 120 {
			return fail("invalid_input", "process.run requires a command within 16 KiB and a timeout of at most 120 seconds")
		}
		raw, _ = json.Marshal(input)
	default:
		return fail("capability_denied", "Unknown resource method")
	}
	if (method == "file.read" || method == "file.write") && (path == "" || len(path) > 4096 || strings.ContainsRune(path, 0)) {
		return fail("invalid_input", "File resource requires a bounded path")
	}

	h.controller.mu.RLock()
	rt := h.controller.Rt
	h.controller.mu.RUnlock()
	if rt == nil || rt.Tools == nil {
		return fail("resource_unavailable", "Runtime resource tool unavailable")
	}
	if before := rt.AgentLoop.Hooks.BeforeToolUse; before != nil {
		decision, err := before(ctx, toolName, raw)
		if err != nil || !decision.Allow {
			return fail("permission_denied", "Host resource approval denied or unresolved")
		}
	}
	if rt.Permission != nil {
		decision := rt.Permission.Check(ctx, rt.AgentID, toolName, raw)
		if decision.Behavior != tool.DecisionAllow {
			return fail("permission_denied", "Host resource policy denied or unresolved")
		}
	}
	if err := ctx.Err(); err != nil {
		return fail("cancelled", "Resource operation cancelled")
	}
	mutationStarted = method == "file.write" || method == "process.run" || method == "network.fetch"
	result, err := rt.Tools.Execute(ctx, toolName, raw)
	if after := rt.AgentLoop.Hooks.AfterToolUse; after != nil {
		after(ctx, toolName, raw, result)
	}
	if err != nil {
		return fail("resource_failed", "Registered resource tool failed")
	}
	if len(result.Images) > 0 || len(result.Output) > 64<<10 || len(result.Error) > 4096 {
		return fail("resource_limit", "Extension resource operations require text output within 64 KiB")
	}
	returnRaw, err := json.Marshal(result)
	if err != nil {
		return fail("resource_failed", "Resource result encoding failed")
	}
	if len(returnRaw) > 128<<10 {
		return fail("resource_limit", "Encoded resource result exceeds 128 KiB; do not automatically retry mutations")
	}
	return returnRaw, nil
}
