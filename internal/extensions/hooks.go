package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/sausheong/hand/extension/protocol"
)

var ErrPolicyDenied = errors.New("extension policy denied action")

type Hook struct {
	ID         string
	Order      int
	Mandatory  bool
	Connection *Connection
}
type Hooks struct {
	entries    []Hook
	capability string
}

// NewHooks snapshots host-owned ordering/mandatory settings. The peer cannot
// choose its policy importance or enable a capability absent from its handshake.
func NewHooks(capability string, entries []Hook) (*Hooks, error) {
	if capability != "context.transform" && capability != "policy.check" {
		return nil, errors.New("invalid hook capability")
	}
	if len(entries) > 16 {
		return nil, errors.New("at most 16 extension hooks are allowed")
	}
	copyEntries := append([]Hook(nil), entries...)
	seen := map[string]bool{}
	for _, e := range copyEntries {
		if e.ID == "" || len(e.ID) > 64 || seen[e.ID] || e.Connection == nil {
			return nil, errors.New("invalid or duplicate hook identity")
		}
		seen[e.ID] = true
		e.Connection.mu.Lock()
		allowed := e.Connection.capabilities[capability]
		e.Connection.mu.Unlock()
		if !allowed {
			return nil, errors.New("hook capability not negotiated")
		}
	}
	sort.Slice(copyEntries, func(i, j int) bool {
		if copyEntries[i].Order != copyEntries[j].Order {
			return copyEntries[i].Order < copyEntries[j].Order
		}
		return copyEntries[i].ID < copyEntries[j].ID
	})
	return &Hooks{entries: copyEntries, capability: capability}, nil
}

type HookDiagnostic struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

// ContextItem is an input contribution, not a chat message. User, system and
// policy items remain host-owned and are never sent to context transforms.
type ContextItem struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Text string `json:"text"`
}
type replacement struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

func validContext(items []ContextItem) error {
	if len(items) > 64 {
		return errors.New("too many transform context items")
	}
	seen := map[string]bool{}
	total := 0
	for _, item := range items {
		if item.ID == "" || len(item.ID) > 64 || seen[item.ID] {
			return errors.New("invalid or duplicate context identity")
		}
		seen[item.ID] = true
		switch item.Kind {
		case "user", "system", "policy", "retrieval", "tool_result":
		default:
			return errors.New("unknown context item kind")
		}
		if len(item.Text) > 16<<10 || !utf8.ValidString(item.Text) || strings.ContainsRune(item.Text, 0) {
			return errors.New("invalid or oversized context text")
		}
		total += len(item.Text)
	}
	if total > 64<<10 {
		return errors.New("transform context exceeds 64 KiB")
	}
	return nil
}
func (h *Hooks) Transform(ctx context.Context, items []ContextItem) ([]ContextItem, []HookDiagnostic, error) {
	if h.capability != "context.transform" {
		return nil, nil, errors.New("wrong hook pipeline")
	}
	if err := validContext(items); err != nil {
		return nil, nil, err
	}
	out := append([]ContextItem(nil), items...)
	diagnostics := []HookDiagnostic{}
	for _, entry := range h.entries {
		if err := ctx.Err(); err != nil {
			return nil, diagnostics, err
		}
		editable := []ContextItem{}
		positions := map[string]int{}
		for i, item := range out {
			if item.Kind == "retrieval" || item.Kind == "tool_result" {
				editable = append(editable, item)
				positions[item.ID] = i
			}
		}
		raw, _ := json.Marshal(struct {
			Items []ContextItem `json:"items"`
		}{editable})
		result, err := entry.Connection.Call(ctx, "context.transform", raw)
		if ctx.Err() != nil {
			return nil, diagnostics, context.Cause(ctx)
		}
		var response struct {
			Replacements []replacement `json:"replacements"`
		}
		if err == nil {
			err = protocol.DecodePayload(result, &response)
		}
		if err == nil && response.Replacements == nil {
			err = errors.New("transform replacements array is required")
		}
		candidate := append([]ContextItem(nil), out...)
		if err == nil {
			seen := map[string]bool{}
			for _, r := range response.Replacements {
				pos, ok := positions[r.ID]
				if !ok || seen[r.ID] {
					err = errors.New("transform changed protected, unknown or duplicate context ID")
					break
				}
				seen[r.ID] = true
				candidate[pos].Text = r.Text
			}
			if err == nil {
				err = validContext(candidate)
			}
		}
		if err != nil {
			diagnostics = append(diagnostics, HookDiagnostic{entry.ID, err.Error()})
			if entry.Mandatory {
				return nil, diagnostics, fmt.Errorf("mandatory context hook %s failed: %w", entry.ID, err)
			}
			continue
		}
		out = candidate
	}
	return out, diagnostics, nil
}

type PolicyRequest struct {
	Input    json.RawMessage `json:"input,omitempty"`
	Action   string          `json:"action"`
	Resource string          `json:"resource"`
}

// Check can only restrict a host-authorised action. An explicit peer denial
// always denies; optional hook errors are disclosed, mandatory errors deny.
func (h *Hooks) Check(ctx context.Context, request PolicyRequest, hostAllowed bool) ([]HookDiagnostic, error) {
	if !hostAllowed {
		return nil, ErrPolicyDenied
	}
	if h.capability != "policy.check" {
		return nil, errors.New("wrong hook pipeline")
	}
	if request.Action == "" || len(request.Action) > 64 || len(request.Resource) > 4096 || !utf8.ValidString(request.Resource) {
		return nil, errors.New("invalid policy request")
	}
	if len(request.Input) > 16<<10 {
		return nil, errors.New("policy tool input exceeds 16 KiB")
	}
	if len(request.Input) > 0 {
		var object map[string]json.RawMessage
		if err := protocol.DecodePayload(request.Input, &object); err != nil {
			return nil, err
		}
	}
	diagnostics := []HookDiagnostic{}
	raw, _ := json.Marshal(request)
	for _, entry := range h.entries {
		if err := ctx.Err(); err != nil {
			return diagnostics, err
		}
		result, err := entry.Connection.Call(ctx, "policy.check", raw)
		if ctx.Err() != nil {
			return diagnostics, context.Cause(ctx)
		}
		var decision struct {
			Allow  *bool  `json:"allow"`
			Reason string `json:"reason"`
		}
		if err == nil {
			err = protocol.DecodePayload(result, &decision)
		}
		if err == nil && (decision.Allow == nil || len(decision.Reason) > 4096 || !utf8.ValidString(decision.Reason)) {
			err = errors.New("invalid extension policy decision")
		}
		if err != nil {
			diagnostics = append(diagnostics, HookDiagnostic{entry.ID, err.Error()})
			if entry.Mandatory {
				return diagnostics, fmt.Errorf("%w: mandatory hook %s failed: %v", ErrPolicyDenied, entry.ID, err)
			}
			continue
		}
		if !*decision.Allow {
			return diagnostics, fmt.Errorf("%w: %s: %s", ErrPolicyDenied, entry.ID, decision.Reason)
		}
	}
	return diagnostics, nil
}
