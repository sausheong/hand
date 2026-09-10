// Package protocol defines Hand's versioned JSONL wire messages. It does not
// depend on terminal rendering or Harness. Version 1 is under development.
package protocol

import (
	"encoding/json"
	"time"
)

const Version = 1
const MaxFrameBytes = 1 << 20 // excludes the terminating newline
const MaxRequestIDBytes = 128

type Request struct {
	Version int             `json:"version"`
	ID      string          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

type Response struct {
	Version   int             `json:"version"`
	RequestID string          `json:"request_id"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *Error          `json:"error,omitempty"`
}
type Event struct {
	Version   int             `json:"version"`
	EventID   string          `json:"event_id"`
	Sequence  uint64          `json:"sequence"`
	SessionID string          `json:"session_id"`
	RunID     string          `json:"run_id"`
	RequestID string          `json:"request_id"`
	Timestamp time.Time       `json:"timestamp"`
	Kind      string          `json:"kind"`
	Payload   json.RawMessage `json:"payload"`
}
