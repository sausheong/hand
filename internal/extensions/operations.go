package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sausheong/hand/extension/protocol"
)

func (m *Manager) begin(parent context.Context) (context.Context, func(), error) {
	return m.beginWithTimeout(parent, protocolTimeout)
}
func (m *Manager) beginWithTimeout(parent context.Context, timeout time.Duration) (context.Context, func(), error) {
	if m.closing.Load() {
		return parent, nil, errors.New("extension manager closed")
	}
	if !m.mu.TryRLock() {
		return parent, nil, ErrBusy
	}
	ctx, release := m.operation(parent, timeout)
	if ctx.Err() != nil {
		release()
		m.mu.RUnlock()
		return parent, nil, context.Cause(ctx)
	}
	return ctx, func() { release(); m.mu.RUnlock() }, nil
}
func (m *Manager) ExecuteCommand(parent context.Context, extension, name, args string) (protocol.Presentation, error) {
	var out protocol.Presentation
	if len(args) > 16<<10 || !utf8.ValidString(args) || strings.ContainsRune(args, 0) {
		return out, errors.New("invalid extension command arguments")
	}
	ctx, release, err := m.beginWithTimeout(parent, interactiveTimeout)
	if err != nil {
		return out, err
	}
	defer release()
	entry, ok := m.entries[extension]
	if !ok {
		return out, errors.New("extension not active")
	}
	registered := false
	for _, command := range entry.hello.Commands {
		if command.Name == name {
			registered = true
			break
		}
	}
	if !registered {
		return out, errors.New("extension command not registered")
	}
	raw, _ := json.Marshal(struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{name, args})
	result, err := entry.connection.Call(ctx, "command.execute", raw)
	if err != nil {
		return out, err
	}
	if err = protocol.DecodePayload(result, &out); err != nil {
		return protocol.Presentation{}, err
	}
	if out.Blocks == nil {
		return protocol.Presentation{}, errors.New("extension command requires presentation blocks")
	}
	if err = out.Validate(); err != nil {
		return protocol.Presentation{}, err
	}
	return out, nil
}

// hookPipeline is called with registry ownership held for its whole execution.
func (m *Manager) hookPipeline(capability string) (*Hooks, error) {
	entries := []Hook{}
	for name, entry := range m.entries {
		for _, c := range entry.hello.Capabilities {
			if c == capability {
				entries = append(entries, Hook{ID: name, Order: entry.spec.Order, Mandatory: entry.spec.Mandatory, Connection: entry.connection})
				break
			}
		}
	}
	return NewHooks(capability, entries)
}
func (m *Manager) Transform(parent context.Context, items []ContextItem) ([]ContextItem, []HookDiagnostic, error) {
	ctx, release, err := m.begin(parent)
	if err != nil {
		return nil, nil, err
	}
	defer release()
	pipeline, err := m.hookPipeline("context.transform")
	if err != nil {
		return nil, nil, err
	}
	return pipeline.Transform(ctx, items)
}
func (m *Manager) CheckPolicy(parent context.Context, request PolicyRequest, hostAllowed bool) ([]HookDiagnostic, error) {
	if !hostAllowed {
		return nil, ErrPolicyDenied
	}
	ctx, release, err := m.begin(parent)
	if err != nil {
		return nil, err
	}
	defer release()
	pipeline, err := m.hookPipeline("policy.check")
	if err != nil {
		return nil, err
	}
	return pipeline.Check(ctx, request, true)
}

// NotifyLifecycle reports delivery errors; callers must not rewrite a previously
// terminal run outcome based on an observer's response.
func (m *Manager) NotifyLifecycle(parent context.Context, event string, data json.RawMessage) ([]HookDiagnostic, error) {
	switch event {
	case "session.open", "session.close", "run.start", "run.finish", "context.compacted":
	default:
		return nil, errors.New("unknown extension lifecycle event")
	}
	if len(data) > 16<<10 {
		return nil, errors.New("extension lifecycle data exceeds 16 KiB")
	}
	var object map[string]json.RawMessage
	if err := protocol.DecodePayload(data, &object); err != nil {
		return nil, err
	}
	timeout := protocolTimeout
	if event == "run.start" {
		timeout = interactiveTimeout
	}
	ctx, release, err := m.beginWithTimeout(parent, timeout)
	if err != nil {
		return nil, err
	}
	defer release()
	entries := []managed{}
	for _, entry := range m.entries {
		for _, subscription := range entry.hello.Subscriptions {
			if subscription == event {
				entries = append(entries, entry)
				break
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].spec.Order != entries[j].spec.Order {
			return entries[i].spec.Order < entries[j].spec.Order
		}
		return entries[i].spec.Name < entries[j].spec.Name
	})
	params, _ := json.Marshal(struct {
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}{event, data})
	diagnostics := []HookDiagnostic{}
	for _, entry := range entries {
		result, err := entry.connection.Call(ctx, "lifecycle.notify", params)
		if ctx.Err() != nil {
			return diagnostics, context.Cause(ctx)
		}
		if err == nil {
			var ack struct{}
			err = protocol.DecodePayload(result, &ack)
		}
		if err != nil {
			diagnostics = append(diagnostics, HookDiagnostic{entry.spec.Name, err.Error()})
			if entry.spec.Mandatory {
				return diagnostics, fmt.Errorf("mandatory lifecycle observer %s failed: %w", entry.spec.Name, err)
			}
		}
	}
	return diagnostics, nil
}

// ContextTransformEnabled inspects the negotiated registry without treating a
// closed host as an empty optional pipeline.
func (m *Manager) ContextTransformEnabled() (bool, error) {
	return m.CapabilityEnabled("context.transform")
}
func (m *Manager) CapabilityEnabled(wanted string) (bool, error) {
	if m.closing.Load() {
		return false, errors.New("extension manager closed")
	}
	if !m.mu.TryRLock() {
		return false, ErrBusy
	}
	defer m.mu.RUnlock()
	for _, entry := range m.entries {
		for _, capability := range entry.hello.Capabilities {
			if capability == wanted {
				return true, nil
			}
		}
	}
	return false, nil
}
