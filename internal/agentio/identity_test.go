package agentio_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/permissions"
)

type cancellingApprovalSender struct {
	t      *testing.T
	id     agentio.RunIdentity
	cancel context.CancelFunc
}

func (s cancellingApprovalSender) Send(msg any) {
	req, ok := msg.(agentio.ApprovalRequest)
	if !ok {
		s.t.Fatalf("unexpected message %T", msg)
	}
	if req.Identity != s.id {
		s.t.Fatalf("approval identity %+v, want %+v", req.Identity, s.id)
	}
	// Both select cases are ready before the hook sees the decision. Even if
	// its decision branch wins, cancellation must prevent a persistent grant.
	s.cancel()
	req.Respond <- agentio.DecisionAlways
}

func TestCancelledApprovalCannotPersistQueuedGrant(t *testing.T) {
	id := agentio.RunIdentity{SessionID: "session", RunID: 7, Generation: 9}
	ctx, cancel := context.WithCancel(agentio.WithRunIdentity(context.Background(), id))
	defer cancel()
	perms := permissions.NewEmptyStore(permissions.DefaultPath(t.TempDir()))
	sender := cancellingApprovalSender{t: t, id: id, cancel: cancel}
	hook := agentio.NewApprovalHook(sender, perms, t.TempDir(), nil, nil)
	decision, err := hook(ctx, "bash", json.RawMessage(`{}`))
	if err != nil || decision.Allow || perms.IsAlwaysAllowed("bash") {
		t.Fatalf("cancelled grant was applied: %+v %v", decision, err)
	}
}
