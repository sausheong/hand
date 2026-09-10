package sdk

import (
	"encoding/json"
	"github.com/sausheong/hand/protocol"
	"strings"
	"testing"
	"time"
)

func snapshotFixture(t *testing.T) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(protocol.Event{Version: 1, EventID: "event", Sequence: 1, SessionID: "session", RunID: "run", RequestID: "request", Timestamp: time.Now(), Kind: "terminal", Payload: json.RawMessage(`{"status":"completed","text":"answer","reason":"stop","verified":false}`)})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func TestExecutionSnapshotsOwnPayload(t *testing.T) {
	event := snapshotFixture(t)
	raw := json.RawMessage(`{"id":"request","session_id":"session","run_id":"run","state":"completed","result":` + string(event) + `}`)
	execution, err := decodeExecution(raw)
	if err != nil {
		t.Fatal(err)
	}
	terminal, ok := execution.Terminal()
	if !ok || terminal.Status() != "completed" || terminal.Text() != "answer" || terminal.Reason() != "stop" {
		t.Fatal("terminal fields missing")
	}
	for i := range raw {
		raw[i] = 'x'
	}
	payload := terminal.Payload()
	for i := range payload {
		payload[i] = 'x'
	}
	again, _ := execution.Terminal()
	if !strings.Contains(string(again.Payload()), "completed") {
		t.Fatal("payload shares mutable bytes")
	}
}
func TestExecutionRejectsMismatchedTerminal(t *testing.T) {
	event := snapshotFixture(t)
	_, err := decodeExecution(json.RawMessage(`{"id":"other","session_id":"session","run_id":"run","state":"completed","result":` + string(event) + `}`))
	if err == nil {
		t.Fatal("accepted mismatched terminal")
	}
}
func TestPromptRequestValidation(t *testing.T) {
	for _, pair := range [][2]string{{"", "hello"}, {"id", " "}, {"id", strings.Repeat("x", 65537)}, {"id", string([]byte{255})}, {string([]byte{255}), "hello"}} {
		if _, err := NewPromptRequest(pair[0], pair[1]); err == nil {
			t.Fatalf("accepted invalid request %q", pair[0])
		}
	}
	r, err := NewPromptRequest("id", "hello")
	if err != nil || r.ID() != "id" || r.Text() != "hello" {
		t.Fatal("valid request rejected")
	}
}
