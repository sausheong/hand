package app

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/permissions"
	"github.com/sausheong/harness/runtime"
)

type serviceContextKey struct{}
type approvalWait struct {
	identity agentio.RunIdentity
	ctx      context.Context
	response chan agentio.Decision
	answered bool
}

var ErrApprovalExpired = errors.New("approval is no longer pending for this run")

// RespondApproval accepts each decision once, for the full run identity and
// opaque request ID. Display tool names/inputs are never authorisation tokens.
func (s *Service) RespondApproval(identity agentio.RunIdentity, id string, decision agentio.Decision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.approvals[id]
	if p == nil || p.identity != identity || p.ctx.Err() != nil || p.answered || !s.active || s.state == Cancelling {
		return ErrApprovalExpired
	}
	if decision != agentio.DecisionDeny && decision != agentio.DecisionOnce && decision != agentio.DecisionAlways {
		return errors.New("invalid approval decision")
	}
	p.answered = true
	p.response <- decision
	return nil
}

func (s *Service) requestApproval(ctx context.Context, req agentio.ApprovalRequest) (agentio.Decision, error) {
	identity := agentio.IdentityFromContext(ctx)
	s.mu.Lock()
	if !s.active || (s.state != Running && s.state != AwaitingApproval) || identity.SessionID != s.options.SessionID || identity.RunID != s.runID || identity.Generation != s.runID || ctx.Err() != nil {
		s.mu.Unlock()
		return agentio.DecisionDeny, ErrApprovalExpired
	}
	if len(s.approvals) >= EventQueueCapacity {
		s.mu.Unlock()
		return agentio.DecisionDeny, errors.New("too many pending approvals")
	}
	id := rand.Text()
	p := &approvalWait{identity: identity, ctx: ctx, response: make(chan agentio.Decision, 1)}
	s.approvals[id] = p
	s.state = AwaitingApproval
	s.mu.Unlock()
	remove := func() {
		s.mu.Lock()
		delete(s.approvals, id)
		if len(s.approvals) == 0 && s.state == AwaitingApproval {
			s.state = Running
		}
		s.mu.Unlock()
	}
	defer remove()
	event := Event{Kind: "approval_required", ApprovalID: id, Text: req.Preview, Details: Details{ToolPresent: true, ToolName: req.Tool, ToolInput: string(req.Input)}}
	select {
	case s.control <- event:
	case <-ctx.Done():
		return agentio.DecisionDeny, ctx.Err()
	}
	var decision agentio.Decision
	select {
	case decision = <-p.response:
	case <-ctx.Done():
		return agentio.DecisionDeny, ctx.Err()
	}
	if ctx.Err() != nil {
		return agentio.DecisionDeny, ctx.Err()
	}
	remove()
	select {
	case s.control <- Event{Kind: "approval_resolved", ApprovalID: id}:
	case <-ctx.Done():
		return agentio.DecisionDeny, ctx.Err()
	}
	return decision, nil
}

type approvalSender func(any)

func (f approvalSender) Send(msg any) { f(msg) }

// NewApprovalHook keeps operation classification/persistence in agentio while
// the active application owns request lifetime and transport. No UI dependency.
func NewApprovalHook(perms *permissions.Store, workspace string, servers []string, trusted map[string]bool) func(context.Context, string, json.RawMessage) (runtime.HookDecision, error) {
	return func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
		s, _ := ctx.Value(serviceContextKey{}).(*Service)
		if s == nil {
			return runtime.HookDecision{}, errors.New("approval requires an application operation")
		}
		var requestErr error
		sender := approvalSender(func(msg any) {
			switch req := msg.(type) {
			case agentio.ApprovalRequest:
				decision, err := s.requestApproval(ctx, req)
				requestErr = err
				req.Respond <- decision
			case agentio.ApprovalWarning:
				select {
				case s.control <- Event{Kind: "warning", Text: req.Message}:
				case <-ctx.Done():
				}
			}
		})
		decision, err := agentio.NewApprovalHook(sender, perms, workspace, servers, trusted)(ctx, name, input)
		if requestErr != nil {
			return runtime.HookDecision{Allow: false}, requestErr
		}
		return decision, err
	}
}

// NewScopedApprovalHook connects scoped authority to the shared application broker.
func NewScopedApprovalHook(authority *permissions.Authority, workspace, digest string, servers []string) func(context.Context, string, json.RawMessage) (runtime.HookDecision, error) {
	return NewBoundApprovalHook(func() PermissionState { return PermissionState{Authority: authority, Digest: digest} }, workspace, servers)
}

func NewBoundApprovalHook(read func() PermissionState, workspace string, servers []string) func(context.Context, string, json.RawMessage) (runtime.HookDecision, error) {
	return func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
		s, _ := ctx.Value(serviceContextKey{}).(*Service)
		if s == nil {
			return runtime.HookDecision{}, errors.New("approval requires an application operation")
		}
		var requestErr error
		sender := approvalSender(func(msg any) {
			switch req := msg.(type) {
			case agentio.ApprovalRequest:
				decision, err := s.requestApproval(ctx, req)
				requestErr = err
				req.Respond <- decision
			case agentio.ApprovalWarning:
				select {
				case s.control <- Event{Kind: "warning", Text: req.Message}:
				case <-ctx.Done():
				}
			}
		})
		state := read()
		decision, err := agentio.NewScopedApprovalHook(sender, state.Authority, workspace, state.Digest, servers)(ctx, name, input)
		if requestErr != nil {
			return runtime.HookDecision{Allow: false}, requestErr
		}
		return decision, err
	}
}
