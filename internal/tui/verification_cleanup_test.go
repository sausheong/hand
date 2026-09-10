package tui

import (
	"github.com/sausheong/hand/internal/verification"
	"strings"
	"testing"
)

func TestVerificationEvidencePageAndConfirmation(t *testing.T) {
	text := strings.Join(evidencePageLines(verification.RecordPage{IDs: []string{"one"}, Next: 32, Total: 33}), "\n")
	for _, want := range []string{"/verify-list 32", "do not prove tests passed", "/verify-delete ID confirm"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
	m, _, _ := sessionUIFixture(t)
	m.evidenceIDs = []string{"selected"}
	for _, args := range [][]string{{"selected"}, {"selected", "yes"}, {"other", "confirm"}} {
		if cmd := m.runVerificationDelete(args); cmd != nil {
			t.Fatal("invalid deletion confirmation dispatched", args)
		}
	}
	m.running = true
	if cmd := m.handleCommand("/verify-delete selected confirm"); cmd != nil {
		t.Fatal("busy deletion dispatched")
	}
	m.running = false
	cmd := m.runVerificationDelete([]string{"selected", "confirm"})
	if cmd == nil || len(m.evidenceIDs) != 0 {
		t.Fatal("deletion selection not consumed")
	}
	m.Update(cmd())
	if m.sessionChanging || len(m.evidenceIDs) != 0 {
		t.Fatal("failed deletion left reusable selection")
	}
}
