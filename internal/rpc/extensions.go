package rpc

import (
	"context"
	"encoding/json"
	"errors"

	extensionprotocol "github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/hand/protocol"
)

func (d *Dispatcher) extensionControl(r protocol.Request) (any, error) {
	if d.controller == nil || d.controller.Extensions == nil {
		return nil, errors.New("extensions unavailable")
	}
	host := d.controller.Extensions
	if r.Method == "extension.questions" {
		var p struct{}
		if err := decodeParams(r.Params, &p); err != nil {
			return nil, err
		}
		return host.Pending(), nil
	}
	var p struct {
		Token  string                   `json:"token"`
		Answer extensionprotocol.Answer `json:"answer"`
	}
	if err := decodeParams(r.Params, &p); err != nil {
		return nil, err
	}
	if p.Token == "" {
		return nil, errors.New("question token required")
	}
	if err := host.Answer(p.Token, p.Answer); err != nil {
		return nil, err
	}
	return map[string]bool{"answered": true}, nil
}

// startExtension persists intent before dispatch and runs outside the dispatcher
// mutex so the client can poll questions, answer, or cancel the owning command.
func (d *Dispatcher) startExtension(r protocol.Request) (RequestRecord, error) {
	if d.controller == nil || d.controller.Extensions == nil {
		return RequestRecord{}, errors.New("extensions unavailable")
	}
	host := d.controller.Extensions
	var executeOperation func(context.Context) (any, error)
	if r.Method == "extension.reload" {
		var p struct {
			Path   string `json:"path"`
			Digest string `json:"digest"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return RequestRecord{}, err
		}
		executeOperation = func(ctx context.Context) (any, error) {
			report, err := host.ReloadFile(ctx, p.Path, p.Digest)
			value := map[string]any{"reload": report, "completed": err == nil}
			if err != nil {
				value["error"] = err.Error()
			}
			return value, nil
		}
	} else {
		var p struct {
			Name      string `json:"name"`
			Command   string `json:"command"`
			Arguments string `json:"arguments"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return RequestRecord{}, err
		}
		if p.Name == "" || p.Command == "" {
			return RequestRecord{}, errors.New("extension and command names required")
		}
		executeOperation = func(ctx context.Context) (any, error) { return host.Execute(ctx, p.Name, p.Command, p.Arguments) }
	}
	record, execute, err := d.beginControl(r)
	if err != nil || !execute {
		return record, err
	}
	if d.extensionCancel != nil {
		response := protocol.Response{Version: protocol.Version, RequestID: r.ID, Error: &protocol.Error{Code: "busy", Message: "extension command already active"}}
		raw, _ := json.Marshal(response)
		if err = d.ledger.Complete(r.ID, raw); err != nil {
			return record, err
		}
		return d.ledger.Lookup(r.ID)
	}
	ctx, cancel := context.WithCancel(d.ctx)
	d.extensionCancel = cancel
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer cancel()
		presentation, runErr := executeOperation(ctx)
		response := protocol.Response{Version: protocol.Version, RequestID: r.ID}
		var encodeErr error
		if runErr != nil {
			response.Error = &protocol.Error{Code: "extension_failed", Message: runErr.Error()}
		} else {
			response.Result, encodeErr = json.Marshal(presentation)
		}
		raw, marshalErr := json.Marshal(response)
		d.mu.Lock()
		defer d.mu.Unlock()
		err := errors.Join(encodeErr, marshalErr)
		if err == nil {
			err = d.ledger.Complete(r.ID, raw)
		}
		if err != nil {
			d.failure = "extension result persistence failed: " + err.Error()
		}
		d.extensionCancel = nil
	}()
	return d.ledger.Lookup(r.ID)
}
