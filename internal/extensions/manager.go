package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sausheong/hand/extension/protocol"
)

var ErrBusy = errors.New("extension manager has active work")

// Specification comes from trusted activation metadata. Digest identifies the
// executable/package plus launch configuration; the factory must verify it and
// obtain execution approval. Merely constructing a specification grants nothing.
type Specification struct {
	Name         string   `json:"name"`
	Digest       string   `json:"digest"`
	Capabilities []string `json:"capabilities"`
	Order        int      `json:"order"`
	Mandatory    bool     `json:"mandatory"`
}

// Factory starts a fresh, not-yet-initialized admitted connection. Startup must
// respect operation cancellation; process ownership follows lifetime. Startup
// effects outside the registry cannot be rolled back by this manager.
type Factory func(operation, lifetime context.Context, spec Specification) (*Connection, error)
type managed struct {
	spec       Specification
	hello      protocol.Hello
	connection *Connection
}
type ReloadReport struct {
	Committed bool     `json:"committed"`
	Started   []string `json:"started"`
	Reused    []string `json:"reused"`
	Removed   []string `json:"removed"`
}
type Manager struct {
	mu      sync.RWMutex
	entries map[string]managed
	factory Factory
	ctx     context.Context
	cancel  context.CancelFunc
	closing atomic.Bool
}

func NewManager(parent context.Context, factory Factory) (*Manager, error) {
	if factory == nil {
		return nil, errors.New("extension factory unavailable")
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	return &Manager{entries: map[string]managed{}, factory: factory, ctx: ctx, cancel: cancel}, nil
}
func (m *Manager) operation(parent context.Context, timeout time.Duration) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	detach := context.AfterFunc(m.ctx, cancel)
	if m.ctx.Err() != nil {
		cancel()
	}
	return ctx, func() { detach(); cancel() }
}
func normalizeSpecifications(input []Specification) ([]Specification, error) {
	if len(input) > 16 {
		return nil, errors.New("at most 16 active extensions are allowed")
	}
	out := append([]Specification(nil), input...)
	seen := map[string]bool{}
	for i := range out {
		s := &out[i]
		if err := (protocol.Hello{Version: 1, Name: s.Name}).Validate(nil); err != nil {
			return nil, err
		}
		if seen[s.Name] {
			return nil, errors.New("duplicate extension name")
		}
		seen[s.Name] = true
		if len(s.Digest) != 64 || strings.IndexFunc(s.Digest, func(r rune) bool { return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') }) >= 0 {
			return nil, errors.New("extension launch digest must be lowercase SHA-256")
		}
		if err := protocol.ValidateCapabilities(s.Capabilities); err != nil {
			return nil, err
		}
		s.Capabilities = append([]string(nil), s.Capabilities...)
		sort.Strings(s.Capabilities)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Reload validates all replacements before swapping the active registry. It
// refuses active work rather than interrupting a command to reload its peer.
func (m *Manager) Reload(parent context.Context, input []Specification) (ReloadReport, error) {
	return m.ReloadWithFactory(parent, input, nil)
}

// ReloadWithFactory stages a new admitted factory and commits it only with the
// registry. Nil retains the current factory. Callers must preserve package identity.
func (m *Manager) ReloadWithFactory(parent context.Context, input []Specification, replacement Factory) (report ReloadReport, err error) {
	specs, err := normalizeSpecifications(input)
	if err != nil {
		return report, err
	}
	if m.closing.Load() {
		return report, errors.New("extension manager closed")
	}
	if !m.mu.TryLock() {
		return report, ErrBusy
	}
	defer m.mu.Unlock()
	ctx, release := m.operation(parent, protocolTimeout)
	defer release()
	if ctx.Err() != nil {
		return report, context.Cause(ctx)
	}
	launch := m.factory
	if replacement != nil {
		launch = replacement
	}
	next := map[string]managed{}
	staged := []*Connection{}
	defer func() {
		if !report.Committed {
			for _, c := range staged {
				err = errors.Join(err, c.Close())
			}
		}
	}()
	for _, spec := range specs {
		if old, ok := m.entries[spec.Name]; ok && reflect.DeepEqual(old.spec, spec) {
			_, _, _, _, health := old.connection.Status()
			if health == nil {
				next[spec.Name] = old
				report.Reused = append(report.Reused, spec.Name)
				continue
			}
		}
		connection, startErr := launch(ctx, m.ctx, spec)
		if connection != nil {
			for _, active := range m.entries {
				if active.connection == connection {
					return report, errors.New("factory reused an active extension connection")
				}
			}
			for _, prepared := range staged {
				if prepared == connection {
					return report, errors.New("factory reused a staged extension connection")
				}
			}
			staged = append(staged, connection)
		}
		if startErr != nil {
			return report, fmt.Errorf("start extension %s: %w", spec.Name, startErr)
		}
		if connection == nil {
			return report, errors.New("factory returned no extension connection")
		}
		hello, initErr := connection.Initialize(ctx, spec.Name, spec.Capabilities)
		if initErr != nil {
			return report, initErr
		}
		declared := append([]string(nil), hello.Capabilities...)
		sort.Strings(declared)
		if !reflect.DeepEqual(declared, spec.Capabilities) {
			return report, errors.New("extension capabilities differ from reviewed specification")
		}
		next[spec.Name] = managed{spec: spec, hello: hello, connection: connection}
		report.Started = append(report.Started, spec.Name)
	}
	if ctx.Err() != nil || m.closing.Load() {
		return report, context.Canceled
	}
	for name, entry := range next {
		_, _, _, _, health := entry.connection.Status()
		if health != nil {
			return report, fmt.Errorf("extension %s became unhealthy during staging: %w", name, health)
		}
	}
	old := m.entries
	m.entries = next
	m.factory = launch
	report.Committed = true
	names := make([]string, 0, len(old))
	for name := range old {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		previous := old[name]
		if replacement, ok := next[name]; ok && replacement.connection == previous.connection {
			continue
		}
		if _, ok := next[name]; !ok {
			report.Removed = append(report.Removed, name)
		}
		err = errors.Join(err, previous.connection.Close())
	}
	return report, err
}

// Call keeps the active registry stable until the request and callbacks join.
// Method-specific public APIs must additionally enforce negotiated capabilities.
func (m *Manager) Call(parent context.Context, name, method string, params json.RawMessage) (json.RawMessage, error) {
	if m.closing.Load() {
		return nil, errors.New("extension manager closed")
	}
	if !m.mu.TryRLock() {
		return nil, ErrBusy
	}
	defer m.mu.RUnlock()
	ctx, release := m.operation(parent, requestTimeout(method, params))
	defer release()
	entry, ok := m.entries[name]
	if !ok {
		return nil, errors.New("extension not active")
	}
	return entry.connection.Call(ctx, method, params)
}

type RegisteredCommand struct {
	Extension   string `json:"extension"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (m *Manager) Commands() []RegisteredCommand {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []RegisteredCommand{}
	for name, entry := range m.entries {
		for _, command := range entry.hello.Commands {
			out = append(out, RegisteredCommand{name, command.Name, command.Description})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Extension != out[j].Extension {
			return out[i].Extension < out[j].Extension
		}
		return out[i].Name < out[j].Name
	})
	return out
}
func (m *Manager) Close() error {
	m.closing.Store(true)
	m.cancel()
	m.mu.Lock()
	defer m.mu.Unlock()
	names := []string{}
	for name := range m.entries {
		names = append(names, name)
	}
	sort.Strings(names)
	var err error
	for _, name := range names {
		err = errors.Join(err, m.entries[name].connection.Close())
		delete(m.entries, name)
	}
	return err
}
