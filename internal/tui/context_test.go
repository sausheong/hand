package tui

import (
	"github.com/sausheong/harness/runtime"
	"strings"
	"testing"
)

func TestContextCommandAndEstimateDisclosure(t *testing.T) {
	lines := contextInspectionLines(runtime.ContextInspection{EstimatedTokens: 42, EstimateMethod: "estimate, not provider usage", Contributions: []runtime.ContextContribution{{Name: "Messages", Count: 2, EstimatedTokens: 12}}, Limitations: []string{"excludes next input"}})
	text := strings.Join(lines, "\n")
	for _, want := range []string{"Estimated context: 42", "not provider usage", "12 estimated tokens (2 items)", "excludes next input"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
	m, _, _ := sessionUIFixture(t)
	if cmd := m.handleCommand("/context extra"); cmd != nil {
		t.Fatal("invalid arguments dispatched")
	}
	m.running = true
	if cmd := m.handleCommand("/context"); cmd != nil {
		t.Fatal("busy inspection dispatched")
	}
	m.running = false
	cmd := m.handleCommand("/context")
	if cmd == nil {
		t.Fatal("inspection not dispatched")
	}
	m.Update(cmd())
	if m.sessionChanging {
		t.Fatal("worker not joined")
	}
}
