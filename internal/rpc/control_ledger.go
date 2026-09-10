package rpc

import (
	"crypto/rand"
	"github.com/sausheong/hand/protocol"
)

func durableControl(method string) bool {
	switch method {
	case "extension.answer", "context.state.replace", "budget.run.select", "budget.run.tokens.decide", "budget.run.cost.decide", "budget.run.time.decide", "budget.time.decide", "budget.cost.decide", "budget.prices.set", "budget.tokens.decide", "summarizer.select", "context.pin", "context.unpin", "skills.reload", "verification.delete", "checkpoint.recovery_resolve", "checkpoint.restore", "permission.revoke", "session.new", "session.select", "session.name", "profile.select", "steer", "followup.enqueue", "queue.edit", "queue.remove", "approval.respond", "cancel", "followup.resume":
		return true
	}
	return false
}

// Replays use the original session scope: session.new/select may themselves have
// changed the selected session. A different method or payload still conflicts.
func (d *Dispatcher) beginControl(request protocol.Request) (RequestRecord, bool, error) {
	sessionID := d.service.Snapshot().SessionID
	if old, err := d.ledger.Lookup(request.ID); err == nil && old.Kind == "control" {
		sessionID = old.SessionID
	}
	record, execute, err := d.ledger.BeginControl(request, sessionID)
	if err != nil || !execute {
		return record, execute, err
	}
	if err = d.ledger.Bind(request.ID, "operation:"+rand.Text()); err != nil {
		return record, false, err
	}
	return record, true, nil
}
