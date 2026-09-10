package app

import (
	"encoding/json"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"testing"
)

func TestWireUsageDistinguishesReportedSubtotalFromCompleteTotal(t *testing.T) {
	for _, tc := range []struct {
		name            string
		details         Details
		known, reported bool
	}{
		{"fully-reported", Details{UsageKnown: true, InputTokens: 40, UsageRequests: 1}, true, true},
		{"mixed", Details{UsageKnown: true, InputTokens: 40, UsageRequests: 2, UsageUnknown: 1}, false, true},
		{"all-unknown", Details{UsageKnown: true, UsageRequests: 2, UsageUnknown: 2}, false, true},
		{"historical-unknown", Details{UsageKnown: true, InputTokens: 40, UsageRequests: 1, UsagePriorUnknown: true}, false, true},
		{"only-historical-unknown", Details{UsagePriorUnknown: true}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event, err := NewWireEvents("request").Encode(Event{Kind: "terminal", Details: tc.details})
			if err != nil {
				t.Fatal(err)
			}
			var payload struct {
				Usage *struct {
					Known    bool  `json:"known"`
					Reported *bool `json:"reported_totals_known"`
					Input    int   `json:"input_tokens"`
					Unknown  int   `json:"unknown_requests"`
					Prior    bool  `json:"prior_unknown"`
				} `json:"usage"`
			}
			if err = json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			u := payload.Usage
			if u == nil || u.Known != tc.known || u.Reported == nil || *u.Reported != tc.reported {
				t.Fatalf("ambiguous usage: %s", event.Payload)
			}
			if u.Input != tc.details.InputTokens || u.Unknown != tc.details.UsageUnknown || u.Prior != tc.details.UsagePriorUnknown {
				t.Fatal("reported subtotal or uncertainty lost")
			}
		})
	}
}

func TestRequestContextBridgePreservesUnknownAndCanonicalInput(t *testing.T) {
	for _, usage := range []*llm.Usage{nil, {InputTokens: 20000, CacheReadInputTokens: 15000}} {
		event, ok := translateHarnessEvent(runtime.AgentEvent{Type: runtime.EventRequestUsage, RequestUsage: &llm.RequestUsage{Usage: usage}})
		if !ok || event.Kind != "context_usage" || event.Done || event.Details.UsageKnown != (usage != nil) {
			t.Fatal("request context lost", event)
		}
		if usage != nil && event.Details.InputTokens != 20000 {
			t.Fatal("cache counted twice", event)
		}
		wire, err := NewWireEvents("request").Encode(Event{Kind: event.Kind, Details: event.Details})
		if err != nil {
			t.Fatal(err)
		}
		var payload struct {
			Usage *struct {
				Known bool `json:"known"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(wire.Payload, &payload); err != nil || payload.Usage == nil || payload.Usage.Known != (usage != nil) {
			t.Fatal("context certainty missing on wire", string(wire.Payload), err)
		}
	}
}
