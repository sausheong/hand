package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/hand/internal/extensions"
)

// ExtensionHost binds admitted peers to application ownership and current session
// state. Its creator must close it before closing the runtime/session. Factory
// admission remains responsible for execution trust and the selected boundary.
type ExtensionHost struct {
	controller        *Controller
	manager           *extensions.Manager
	questions         *extensions.Questions
	identities        map[string]string
	workspace         string
	containerBoundary *extensions.ContainerLaunch
	closing           atomic.Bool
	sessionObserved   atomic.Bool
	closeOnce         sync.Once
	closeErr          error
}

func NewExtensionHost(lifetime context.Context, c *Controller, factory extensions.Factory, identities map[string]string) (*ExtensionHost, error) {
	if c == nil || factory == nil {
		return nil, errors.New("extension controller and admitted factory required")
	}
	host := &ExtensionHost{controller: c, questions: extensions.NewQuestions(), identities: map[string]string{}}
	bound, err := host.bindFactory(factory, identities)
	if err != nil {
		return nil, err
	}
	manager, err := extensions.NewManager(lifetime, bound)
	if err != nil {
		host.questions.Close()
		return nil, err
	}
	host.manager = manager
	for name, identity := range identities {
		host.identities[name] = identity
	}
	return host, nil
}
func (host *ExtensionHost) bindFactory(factory extensions.Factory, identities map[string]string) (extensions.Factory, error) {
	c := host.controller
	owners := map[string]string{}
	for name, identity := range identities {
		if err := (protocol.Hello{Version: 1, Name: name}).Validate(nil); err != nil {
			return nil, err
		}
		if !utf8.ValidString(identity) || strings.TrimSpace(identity) == "" || len(identity) > 256 || strings.ContainsRune(identity, 0) {
			return nil, errors.New("invalid extension package identity")
		}
		owners[name] = identity
	}
	return func(operation, life context.Context, spec extensions.Specification) (*extensions.Connection, error) {
		identity, ok := owners[spec.Name]
		if !ok {
			return nil, errors.New("extension package identity not configured")
		}
		connection, err := factory(operation, life, spec)
		if err != nil {
			return nil, err
		}
		if connection == nil {
			return nil, errors.New("extension factory returned no connection")
		}
		questionHandler := host.questions.Handler(spec.Name)
		err = connection.SetCallbackHandler(func(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, *protocol.Error) {
			if event, ok := ctx.Value(sessionNotificationKey{}).(string); ok && (event != "session.open" || method == "user.question") {
				return nil, &protocol.Error{Code: "callback_unavailable", Message: "Callback unavailable during session notification"}
			}
			if method == "file.read" || method == "file.write" || method == "network.fetch" || method == "process.run" {
				return host.extensionToolResource(ctx, spec, method, params)
			}
			if method == "user.question" {
				return questionHandler(ctx, method, params)
			}
			if method != "state.get" && method != "state.set" {
				return nil, &protocol.Error{Code: "capability_denied", Message: "Host resource callback is unavailable"}
			}
			c.mu.RLock()
			defer c.mu.RUnlock()
			if c.Rt == nil || c.Rt.Session == nil {
				return nil, &protocol.Error{Code: "state_unavailable", Message: "No selected session"}
			}
			state, err := extensions.NewStateStore(c.Rt.Session, identity)
			if err != nil {
				return nil, &protocol.Error{Code: "state_unavailable", Message: err.Error()}
			}
			return state.Handle(ctx, method, params)
		})
		if err != nil {
			connection.Close()
			return nil, err
		}
		return connection, nil
	}, nil
}

func (h *ExtensionHost) Reload(ctx context.Context, specs []extensions.Specification) (extensions.ReloadReport, error) {
	if h.closing.Load() {
		return extensions.ReloadReport{}, errors.New("extension host closed")
	}
	operation, release, err := h.controller.owner().reserve(ctx, Running)
	if err != nil {
		return extensions.ReloadReport{}, err
	}
	defer release()
	return h.manager.Reload(operation, specs)
}
func (h *ExtensionHost) Execute(ctx context.Context, name, command, args string) (protocol.Presentation, error) {
	if h.closing.Load() {
		return protocol.Presentation{}, errors.New("extension host closed")
	}
	operation, release, err := h.controller.owner().reserve(ctx, Running)
	if err != nil {
		return protocol.Presentation{}, err
	}
	defer release()
	return h.manager.ExecuteCommand(operation, name, command, args)
}

// Pending and Answer deliberately do not acquire turn ownership: the command
// waiting for the answer already holds it, keeping session switches and reload
// out until the question and all callbacks have joined.
func (h *ExtensionHost) Pending() []extensions.PendingQuestion { return h.questions.Pending() }
func (h *ExtensionHost) Answer(token string, answer protocol.Answer) error {
	return h.questions.Respond(token, answer)
}
func (h *ExtensionHost) Close() error {
	h.closeOnce.Do(func() {
		h.closing.Store(true)
		h.questions.Close()
		if h.sessionObserved.Load() {
			h.observeSessionTransition(context.Background(), h.controller.SessionID(), "")
		}
		h.closeErr = h.manager.Close()
	})
	return h.closeErr
}
