package extensions

import (
	"encoding/json"
	"time"

	"github.com/sausheong/hand/extension/protocol"
)

const (
	protocolTimeout = 30 * time.Second
	// A complete command or run-start delivery may wait for human input. This
	// ceiling is shared across its callbacks; asking again never resets it.
	interactiveTimeout = 5 * time.Minute
)

func requestTimeout(method string, params json.RawMessage) time.Duration {
	if method == "command.execute" {
		return interactiveTimeout
	}
	if method == "lifecycle.notify" {
		var event struct {
			Event string          `json:"event"`
			Data  json.RawMessage `json:"data"`
		}
		if protocol.DecodePayload(params, &event) == nil && event.Event == "run.start" {
			return interactiveTimeout
		}
	}
	return protocolTimeout
}
