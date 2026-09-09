package extensions

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/sausheong/hand/extension/protocol"
)

// CallbackHandler is trusted host code. It must validate method-specific input,
// apply resource policy, respect context cancellation and join owned work before
// returning. It must not call the same Connection recursively.
type CallbackHandler func(context.Context, string, json.RawMessage) (json.RawMessage, *protocol.Error)

var callbackCapabilities = map[string]string{
	"user.question": "questions", "state.get": "state", "state.set": "state",
	"file.read": "file.read", "file.write": "file.write", "network.fetch": "network.fetch", "process.run": "process.run",
}

// SetCallbackHandler only changes the dispatcher while idle. Capability grants
// come exclusively from the validated handshake, never from this handler.
func (c *Connection) SetCallbackHandler(handler CallbackHandler) error {
	select {
	case c.gate <- struct{}{}:
	default:
		return errors.New("extension request active")
	}
	defer func() { <-c.gate }()
	if c.ctx.Err() != nil {
		return context.Cause(c.ctx)
	}
	c.mu.Lock()
	c.handler = handler
	c.mu.Unlock()
	return nil
}
func (c *Connection) callback(f protocol.Frame) error {
	c.mu.Lock()
	pending := c.pending
	if pending == nil || c.capabilities == nil || f.ID == pending.id || pending.callbacks[f.ID] || len(pending.callbacks) >= 32 {
		c.mu.Unlock()
		return errors.New("unsolicited, duplicate or excessive extension callback")
	}
	pending.callbacks[f.ID] = true
	handler := c.handler
	capability := callbackCapabilities[f.Method]
	allowed := capability != "" && c.capabilities[capability]
	c.mu.Unlock()
	response := protocol.Frame{Version: protocol.Version, Kind: "response", ID: f.ID}
	if !allowed || handler == nil {
		response.Error = &protocol.Error{Code: "capability_denied", Message: "host callback unavailable or capability not approved"}
	} else {
		ctx, cancel := context.WithCancel(pending.ctx)
		detach := context.AfterFunc(c.ctx, cancel)
		if ctx.Err() != nil {
			detach()
			cancel()
			return context.Cause(ctx)
		}
		result, callbackErr := handler(ctx, f.Method, append(json.RawMessage(nil), f.Params...))
		err := ctx.Err()
		detach()
		cancel()
		if err != nil {
			return err
		}
		if callbackErr != nil {
			response.Error = callbackErr
		} else {
			if len(result) == 0 {
				result = json.RawMessage(`null`)
			}
			response.Result = result
		}
	}
	return c.writer.Write(response)
}
