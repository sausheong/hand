package tui

import (
	"context"
	"strings"
	"testing"
)

func TestRunBudgetTerminalDecisionsSelectionAndInspection(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	for _, command := range []string{"/run-budget tokens job 0", "/run-budget tokens job 9223372036854775808", "/run-budget time job 2592001", "/run-budget cost job USD 1 maybe", "/run-budget cost job USD NaN strict", "/run-budget select ../job", "/run-budget tokens job 10 extra"} {
		if cmd := m.handleCommand(command); cmd != nil {
			t.Fatalf("invalid command dispatched %s", command)
		}
	}
	execute := func(command string) sessionChangedMsg {
		t.Helper()
		cmd := m.handleCommand(command)
		if cmd == nil {
			t.Fatalf("missing worker %s", command)
		}
		msg := cmd().(sessionChangedMsg)
		if msg.err != nil {
			t.Fatal(command, msg.err)
		}
		m.Update(msg)
		if m.sessionChanging {
			t.Fatal("worker not released")
		}
		return msg
	}
	execute("/run-budget tokens job 100000")
	execute("/run-budget cost job USD 1.000000001 strict")
	execute("/run-budget time job 600")
	before, err := m.controller.InspectRunBudget(context.Background(), "job")
	if err != nil {
		t.Fatal(err)
	}
	if before.Selected || before.Tokens.Limit != 100000 || before.Cost.LimitNano != 1000000001 || !before.Cost.Strict || !before.Time.Configured {
		t.Fatalf("decisions %+v", before)
	}
	execute("/run-budget select job")
	msg := execute("/run-budget")
	text := strings.Join(msg.lines, "\n")
	if !strings.Contains(text, "Run budget job: selected") || !strings.Contains(text, "Deadline:") || !strings.Contains(text, "prior charges remain") {
		t.Fatal(text)
	}
	after, err := m.controller.InspectRunBudget(context.Background(), "")
	if err != nil || !after.Selected || !after.Time.Deadline.Equal(before.Time.Deadline) {
		t.Fatalf("inspection changed deadline %+v %v", after, err)
	}
	m.running = true
	if cmd := m.handleCommand("/run-budget tokens job 200000"); cmd != nil {
		t.Fatal("active run admitted budget mutation")
	}
	m.running = false
	unchanged, err := m.controller.InspectRunBudget(context.Background(), "job")
	if err != nil || unchanged.Tokens.Limit != 100000 {
		t.Fatalf("busy mutation changed state %+v %v", unchanged, err)
	}
}
