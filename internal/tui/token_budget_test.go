package tui

import (
	"context"
	"github.com/sausheong/hand/internal/app"
	"strings"
	"testing"
)

func TestTokenBudgetTerminalDecisionAndInspection(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	for _, command := range []string{"/budget extra", "/budget-tokens", "/budget-tokens -1", "/budget-tokens 0", "/budget-tokens 999999999999999999999999", "/budget-tokens 100 extra"} {
		if cmd := m.handleCommand(command); cmd != nil {
			t.Fatalf("invalid command dispatched: %s", command)
		}
	}
	m.running = true
	if cmd := m.handleCommand("/budget-tokens 100"); cmd != nil {
		t.Fatal("active run accepted decision")
	}
	m.running = false
	cmd := m.handleCommand("/budget-tokens 100")
	if cmd == nil {
		t.Fatal("decision did not dispatch")
	}
	msg := cmd().(sessionChangedMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	m.Update(msg)
	state, err := m.controller.TokenBudget(context.Background())
	if err != nil || state.Limit != 100 {
		t.Fatalf("state %+v err %v", state, err)
	}
	cmd = m.handleCommand("/budget")
	if cmd == nil {
		t.Fatal("inspection did not dispatch")
	}
	msg = cmd().(sessionChangedMsg)
	if msg.err != nil || !strings.Contains(strings.Join(msg.lines, "\n"), "ceiling: 100") {
		t.Fatalf("inspection %+v", msg)
	}
	m.Update(msg)
	if m.sessionChanging {
		t.Fatal("worker not released")
	}
	text := strings.Join(tokenBudgetLines(app.TokenBudgetView{Limit: 100, Committed: 120, Exhausted: true, Uncertain: 1}), "\n")
	for _, want := range []string{"exhausted", "120", "uncertain or outstanding: 1", "not an additional allowance"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
}
