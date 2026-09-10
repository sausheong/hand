package sdk

import (
	"bytes"
	"encoding/json"
	"testing"
)

// Bound inputs to the public transport frame limit. Accepted snapshots must
// preserve their public identity/outcome projection after canonical encoding.
func FuzzSnapshotDecoders(f *testing.F) {
	f.Add(byte(0), `{"version":1,"event_id":"e","sequence":1,"session_id":"s","run_id":"r","request_id":"q","timestamp":"2026-09-09T00:00:00Z","kind":"terminal","payload":{"status":"completed","reason":"stop","verified":false}}`)
	f.Add(byte(0), `{"version":1,"version":2}`)
	f.Add(byte(0), `{"payload":{"status":"cancelled","status":"completed"}}`)
	f.Add(byte(1), `{"id":"q","session_id":"s","state":"pending"}`)
	f.Add(byte(1), `{"id":"q","session_id":"s","run_id":"r","state":"uncertain"}`)
	f.Add(byte(1), `{"id":"q","session_id":"s","state":"completed","state":"pending"}`)
	f.Fuzz(func(t *testing.T, kind byte, raw string) {
		if len(raw) > 1<<20 {
			return
		}
		if kind%2 == 0 {
			event, err := decodeEvent(json.RawMessage(raw))
			if err != nil {
				return
			}
			encoded, err := json.Marshal(event.wire)
			if err != nil {
				t.Fatal(err)
			}
			again, err := decodeEvent(encoded)
			if err != nil || again.ID() != event.ID() || again.Sequence() != event.Sequence() || !again.Timestamp().Equal(event.Timestamp()) || again.Kind() != event.Kind() || again.Status() != event.Status() || again.Text() != event.Text() || again.Reason() != event.Reason() || !sameSnapshotPayload(again.Payload(), event.Payload()) {
				t.Fatal("accepted event did not preserve snapshot", err)
			}
		} else {
			first, err := decodeExecution(json.RawMessage(raw))
			if err != nil {
				return
			}
			wire := map[string]any{"id": first.ID(), "session_id": first.SessionID(), "run_id": first.RunID(), "state": first.State()}
			if terminal, ok := first.Terminal(); ok {
				wire["result"] = terminal.wire
			}
			encoded, err := json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			second, err := decodeExecution(encoded)
			if err != nil || first.ID() != second.ID() || first.SessionID() != second.SessionID() || first.RunID() != second.RunID() || first.State() != second.State() || first.hasTerminal != second.hasTerminal {
				t.Fatal("execution snapshot unstable", err)
			}
		}
	})
}

func sameSnapshotPayload(a, b json.RawMessage) bool {
	var left, right bytes.Buffer
	if json.Compact(&left, a) != nil || json.Compact(&right, b) != nil {
		return false
	}
	return bytes.Equal(left.Bytes(), right.Bytes())
}
