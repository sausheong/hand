package extensions

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/sausheong/hand/extension/protocol"
)

// Initialize negotiates an explicitly admitted peer. Returned registrations are
// usable only after identity/schema/capability validation succeeds. The caller
// still owns registration collision checks and transactional activation.
func (c *Connection) Initialize(ctx context.Context, name string, approved []string) (protocol.Hello, error) {
	var hello protocol.Hello
	approved = append([]string(nil), approved...)
	if err := protocol.ValidateCapabilities(approved); err != nil {
		return hello, err
	}
	if err := (protocol.Hello{Version: protocol.Version, Name: name}).Validate(nil); err != nil {
		return hello, err
	}
	c.mu.Lock()
	if c.handshakeAttempted {
		c.mu.Unlock()
		return hello, errors.New("extension handshake already attempted")
	}
	c.handshakeAttempted = true
	c.mu.Unlock()
	params, err := json.Marshal(struct {
		Version      int      `json:"version"`
		Capabilities []string `json:"capabilities"`
	}{protocol.Version, approved})
	if err != nil {
		return hello, err
	}
	raw, err := c.Call(ctx, "initialize", params)
	if err == nil {
		err = protocol.DecodePayload(raw, &hello)
	}
	if err == nil && hello.Name != name {
		err = errors.New("extension identity differs from admitted identity")
	}
	if err == nil {
		err = hello.Validate(approved)
	}
	if err != nil {
		c.stop(err)
		<-c.done
		return protocol.Hello{}, err
	}
	c.mu.Lock()
	c.capabilities = make(map[string]bool)
	for _, capability := range hello.Capabilities {
		c.capabilities[capability] = true
	}
	c.mu.Unlock()
	return hello, nil
}
