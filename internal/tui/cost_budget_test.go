package tui

import (
	"context"
	"github.com/sausheong/hand/internal/app"
	"strings"
	"testing"
)

func TestCostAmountExactDecimal(t *testing.T) {
	for input, want := range map[string]int64{"1": 1000000000, "0.000000001": 1, "12.345678901": 12345678901, "1125899.906842624": 1 << 50} {
		actual, err := parseCostAmount(input)
		if err != nil || actual != want || costAmount(actual) != input {
			t.Fatalf("%s => %d %v", input, actual, err)
		}
	}
	for _, input := range []string{"0", "-1", "+1", "1e3", "NaN", "1.", ".1", "1.0000000001", "1125899.906842625", "999999999999999999999", " 1", "1,000"} {
		if _, err := parseCostAmount(input); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
}
func TestCostTerminalExplicitDecisionAndDisplay(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	for _, command := range []string{"/cost USD 1", "/cost USD 1 unknown", "/cost USD -1 strict", "/prices extra"} {
		if cmd := m.handleCommand(command); cmd != nil {
			t.Fatalf("invalid command dispatched: %s", command)
		}
	}
	m.running = true
	if cmd := m.handleCommand("/cost USD 1 strict"); cmd != nil {
		t.Fatal("busy cost change accepted")
	}
	m.running = false
	cmd := m.handleCommand("/cost USD 1.000000001 strict")
	if cmd == nil {
		t.Fatal("missing decision worker")
	}
	msg := cmd().(sessionChangedMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	m.Update(msg)
	view, err := m.controller.CostBudget(context.Background())
	if err != nil || view.LimitNano != 1000000001 || !view.Strict {
		t.Fatalf("view %+v %v", view, err)
	}
	cmd = m.handleCommand("/cost")
	if cmd == nil {
		t.Fatal("missing status worker")
	}
	msg = cmd().(sessionChangedMsg)
	if msg.err != nil || !strings.Contains(strings.Join(msg.lines, "\n"), "1.000000001") {
		t.Fatalf("status %+v", msg)
	}
	m.Update(msg)
	cmd = m.handleCommand("/prices")
	if cmd == nil {
		t.Fatal("missing prices worker")
	}
	msg = cmd().(sessionChangedMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	m.Update(msg)
	if m.sessionChanging {
		t.Fatal("worker not joined")
	}
	text := strings.Join(costBudgetLines(app.CostBudgetView{Currency: "USD", LimitNano: 100, CommittedNano: 200, Exhausted: true, Unpriced: 1}), "\n")
	if !strings.Contains(text, "exhausted") || !strings.Contains(text, "unpriced: 1") || !strings.Contains(text, "not an additional allowance") {
		t.Fatal(text)
	}
}
