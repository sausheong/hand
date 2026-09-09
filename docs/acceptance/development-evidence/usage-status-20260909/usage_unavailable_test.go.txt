package tui

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func TestUsageFailureCannotRetainOtherSessionTotals(t *testing.T) {
	first := session.NewSession("hand", "first")
	record := llm.RequestUsage{ID: "one", Model: "fixture", Category: llm.CallGeneration, Status: "completed", Source: "reported", Usage: &llm.Usage{InputTokens: 40, OutputTokens: 5}}
	if err := sessionio.RecordRequestUsage(first, record); err != nil {
		t.Fatal(err)
	}
	m := NewModel(nil, t.TempDir())
	defer m.CloseApplication()
	m.SetController(&Controller{Rt: &runtime.Runtime{Session: first}})
	if m.usageRequests != 1 || m.sessionUsage.InputTokens != 40 {
		t.Fatal("first session usage not loaded")
	}
	second := session.NewSession("hand", "second")
	if err := second.Annotate("hand.request_usage", json.RawMessage(`{"version":99,"request":{}}`)); err != nil {
		t.Fatal(err)
	}
	m.SetController(&Controller{Rt: &runtime.Runtime{Session: second}})
	if m.identity.SessionID != second.ID {
		t.Fatal("second session not selected")
	}
	if m.usageRequests != 0 || m.usageUnknown != 0 || m.sessionUsage != (llm.Usage{}) || !m.usagePriorUnknown {
		t.Fatal("unavailable accounting retained previous session totals or claimed known usage")
	}
	m.runUsageCommand()
	text := strings.Join(m.transcript, "\n")
	if !strings.Contains(text, "session usage unavailable") || !strings.Contains(text, "Earlier session usage is unknown") {
		t.Fatal("accounting uncertainty not disclosed")
	}
	// A valid later attachment must recover the actual known totals.
	m.SetController(&Controller{Rt: &runtime.Runtime{Session: first}})
	if m.usageRequests != 1 || m.sessionUsage.InputTokens != 40 || m.usagePriorUnknown {
		t.Fatal("valid accounting did not recover")
	}
}

func TestUsageStatusDisclosesIncompleteTotals(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prior   bool
		unknown int
	}{
		{"unmeasured history", true, 0}, {"missing attempt", false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel(nil, t.TempDir())
			defer m.CloseApplication()
			m.sessionUsage = llm.Usage{InputTokens: 40, OutputTokens: 5}
			m.usagePriorUnknown = tc.prior
			m.usageUnknown = tc.unknown
			line := m.usageLine()
			if !strings.Contains(line, "session ≥45 tok (incomplete)") {
				t.Fatalf("uncertainty hidden: %s", line)
			}
			m.usagePriorUnknown = false
			m.usageUnknown = 0
			line = m.usageLine()
			if !strings.Contains(line, "session 45 tok") || strings.Contains(line, "incomplete") {
				t.Fatalf("known totals mislabeled: %s", line)
			}
		})
	}
}

func TestCompactionUsageReadFailureMarksCachedTotalsIncomplete(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	defer m.CloseApplication()
	m.sessionUsage = llm.Usage{InputTokens: 40, OutputTokens: 5}
	m.usageRequests = 1
	m.compacting = true
	m.Update(compactResultMsg{identity: m.identity, generation: m.compactGeneration, usageErr: errors.New("usage journal unreadable")})
	if !m.usagePriorUnknown || m.sessionUsage.InputTokens != 40 || m.usageRequests != 1 {
		t.Fatal("known subtotal must remain visibly incomplete")
	}
	if !strings.Contains(m.usageLine(), "incomplete") {
		t.Fatal("persistent status hides incomplete accounting")
	}
	m.runUsageCommand()
	if !strings.Contains(strings.Join(m.transcript, "\n"), "Earlier session usage is unknown") {
		t.Fatal("usage command hides uncertainty")
	}
}
