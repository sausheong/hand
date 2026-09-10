package agentio

import "context"

// RunIdentity follows asynchronous work across adapters. SessionID identifies
// the conversation, RunID the turn, and Generation invalidates superseded work.
type RunIdentity struct {
	SessionID  string `json:"session_id"`
	RunID      uint64 `json:"run_id"`
	Generation uint64 `json:"generation"`
}
type identityContextKey struct{}

func WithRunIdentity(ctx context.Context, id RunIdentity) context.Context {
	return context.WithValue(ctx, identityContextKey{}, id)
}
func IdentityFromContext(ctx context.Context) RunIdentity {
	id, _ := ctx.Value(identityContextKey{}).(RunIdentity)
	return id
}
