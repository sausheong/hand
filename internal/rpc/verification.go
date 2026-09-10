package rpc

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/sausheong/hand/protocol"
)

// startVerification runs with d.mu held. Intent is durable before dispatch;
// completion is durable before request.get can report a completed response.
func (d *Dispatcher) startVerification(r protocol.Request) (RequestRecord, error) {
	if d.controller == nil {
		return RequestRecord{}, errors.New("verification controller unavailable")
	}
	var p struct {
		Profile   string `json:"profile"`
		Digest    string `json:"digest"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := decodeParams(r.Params, &p); err != nil {
		return RequestRecord{}, err
	}
	if !p.Confirmed {
		return RequestRecord{}, errors.New("explicit verification confirmation required")
	}
	record, execute, err := d.beginControl(r)
	if err != nil {
		return record, err
	}
	if !execute {
		return record, nil
	}
	if d.verificationCancel != nil {
		answer := protocol.Response{Version: protocol.Version, RequestID: r.ID, Error: &protocol.Error{Code: "busy", Message: "verification already active"}}
		raw, _ := json.Marshal(answer)
		if err := d.ledger.Complete(r.ID, raw); err != nil {
			return record, err
		}
		return d.ledger.Lookup(r.ID)
	}
	ctx, cancel := context.WithCancel(d.ctx)
	d.verificationCancel = cancel
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer cancel()
		result, runErr := d.controller.RunNamedVerification(ctx, p.Profile, p.Digest)
		value := map[string]any{"verification": result, "completed": runErr == nil}
		if runErr != nil {
			value["error"] = runErr.Error()
		}
		payload, encodeErr := json.Marshal(value)
		response := protocol.Response{Version: protocol.Version, RequestID: r.ID, Result: payload}
		raw, marshalErr := json.Marshal(response)
		d.mu.Lock()
		defer d.mu.Unlock()
		err := errors.Join(encodeErr, marshalErr)
		if err == nil {
			err = d.ledger.Complete(r.ID, raw)
		}
		if err != nil {
			d.failure = "verification result persistence failed: " + err.Error()
		}
		d.verificationCancel = nil
	}()
	return d.ledger.Lookup(r.ID)
}
