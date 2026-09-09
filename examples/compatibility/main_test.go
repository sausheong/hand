package main

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sausheong/hand/protocol"
)

// Exercise the shipped example itself against a freshly built Hand. The
// example verifies six real interface journeys, exact terminal counts and
// provider cancellation, using only its local deterministic HTTP provider.
func TestCompatibilityAcrossPublicInterfaces(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "hand")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/hand")
	build.Dir = "../.."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Hand: %v\n%s", err, output)
	}
	if err := run(binary); err != nil {
		t.Fatal(err)
	}
}

func TestCompatibilityMissingBinaryCannotReportSuccess(t *testing.T) {
	err := run(filepath.Join(t.TempDir(), "missing-hand"))
	if err == nil || !strings.Contains(err.Error(), "CLI failed") {
		t.Fatalf("missing binary did not fail CLI journey: %v", err)
	}
}

func TestCLIObservationRejectsMalformedEventsWithoutChangingResult(t *testing.T) {
	valid := protocol.Event{Version: protocol.Version, RequestID: "request", RunID: "run", Kind: "terminal", Payload: json.RawMessage(`{"status":"cancelled"}`)}
	for _, mutate := range []func(*protocol.Event){
		func(e *protocol.Event) { e.Version = 0 },
		func(e *protocol.Event) { e.RequestID = "" },
		func(e *protocol.Event) { e.RunID = "" },
		func(e *protocol.Event) { e.Kind = "" },
		func(e *protocol.Event) { e.Payload = json.RawMessage(`{"status":42}`) },
	} {
		event := valid
		mutate(&event)
		raw, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		before := observation{Text: "existing", Status: "completed", Terminals: 1}
		after := before
		if _, err = observeCLI(&after, raw); err == nil || after != before {
			t.Fatalf("invalid event affected result: %s %+v %v", raw, after, err)
		}
	}
	for _, raw := range []string{`{`, `null`, `{} {}`} {
		var result observation
		if _, err := observeCLI(&result, []byte(raw)); err == nil || result != (observation{}) {
			t.Fatal("malformed JSON counted", raw)
		}
	}
	raw, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	var result observation
	for i := 0; i < 2; i++ {
		if kind, err := observeCLI(&result, raw); err != nil || kind != "terminal" {
			t.Fatal(kind, err)
		}
	}
	if result.Terminals != 2 || result.Status != "cancelled" {
		t.Fatal("duplicate terminals hidden", result)
	}
}
