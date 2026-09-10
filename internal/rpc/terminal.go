package rpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/protocol"
)

func validateTerminal(record RequestRecord) error {
	fields, err := ledgerObject(record.Result)
	if err != nil {
		return err
	}
	for key := range fields {
		switch key {
		case "version", "event_id", "sequence", "session_id", "run_id", "request_id", "timestamp", "kind", "payload":
		default:
			return errors.New("unknown terminal envelope field")
		}
	}
	var event protocol.Event
	if err := json.Unmarshal(record.Result, &event); err != nil {
		return err
	}
	if event.Version != protocol.Version || event.EventID == "" || event.Sequence == 0 || event.Timestamp.IsZero() || event.Kind != "terminal" || event.SessionID != record.SessionID || event.RunID != record.RunID || event.RequestID != record.ID {
		return errors.New("invalid terminal identity or kind")
	}
	payload, err := ledgerObject(event.Payload)
	if err != nil {
		return err
	}
	for key := range payload {
		for _, canonical := range []string{"status", "reason", "verified"} {
			if key != canonical && strings.EqualFold(key, canonical) {
				return errors.New("noncanonical terminal outcome field")
			}
		}
	}
	var status agentio.RunStatus
	var reason string
	var verified bool
	if err := json.Unmarshal(payload["status"], &status); err != nil {
		return err
	}
	if err := json.Unmarshal(payload["reason"], &reason); err != nil {
		return err
	}
	flag := bytes.TrimSpace(payload["verified"])
	if !bytes.Equal(flag, []byte("true")) && !bytes.Equal(flag, []byte("false")) {
		return errors.New("terminal verification must be an explicit boolean")
	}
	if err := json.Unmarshal(flag, &verified); err != nil {
		return err
	}
	if reason == "" || (verified && status != agentio.Completed) {
		return errors.New("invalid terminal outcome")
	}
	switch status {
	case agentio.Completed, agentio.VerificationFailed, agentio.Cancelled, agentio.BudgetExhausted, agentio.InfrastructureError:
		return nil
	default:
		return errors.New("unknown terminal outcome")
	}
}
