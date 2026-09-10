package sdk

import (
	"encoding/json"
	"github.com/sausheong/hand/protocol"
	"testing"
)

func TestTerminalSnapshotRequiresOutcome(t *testing.T) {
	for _, payload := range []string{`{}`, `{"status":"completed","reason":"stop"}`, `{"status":"invented","reason":"stop","verified":false}`, `{"status":"completed","reason":null,"verified":false}`, `{"status":"completed","reason":"stop","verified":null}`, `{"status":"cancelled","reason":"user","verified":true}`} {
		var event protocol.Event
		if err := json.Unmarshal(snapshotFixture(t), &event); err != nil {
			t.Fatal(err)
		}
		event.Payload = json.RawMessage(payload)
		raw, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := decodeEvent(raw); err == nil {
			t.Errorf("invalid outcome accepted: %s", payload)
		}
	}
	for _, status := range []string{"completed", "cancelled", "verification_failed", "budget_exhausted", "infrastructure_error"} {
		var event protocol.Event
		if err := json.Unmarshal(snapshotFixture(t), &event); err != nil {
			t.Fatal(err)
		}
		event.Payload = json.RawMessage(`{"status":"` + status + `","reason":"fixture","verified":false}`)
		raw, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		if decoded, err := decodeEvent(raw); err != nil || decoded.Status() != status {
			t.Fatal("valid terminal rejected", status, err)
		}
	}
}
