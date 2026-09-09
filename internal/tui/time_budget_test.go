package tui

import (
	"context"
	"github.com/sausheong/hand/internal/app"
	"strings"
	"testing"
)

func TestTimeBudgetTerminalDecisionAndStatus(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	for _, command := range []string{"/time-budget 0", "/time-budget -1", "/time-budget 2592001", "/time-budget 9223372036854775808", "/time-budget 10 extra"} {
		if cmd := m.handleCommand(command); cmd != nil {
			t.Fatalf("invalid allowance dispatched: %s", command)
		}
	}
	m.running = true
	if cmd := m.handleCommand("/time-budget 60"); cmd != nil {
		t.Fatal("busy decision accepted")
	}
	m.running = false
	cmd := m.handleCommand("/time-budget 60")
	if cmd == nil {
		t.Fatal("missing decision worker")
	}
	msg := cmd().(sessionChangedMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	m.Update(msg)
	before, err := m.controller.TimeBudget(context.Background())
	if err != nil || !before.Configured || before.Expired {
		t.Fatalf("state %+v %v", before, err)
	}
	cmd = m.handleCommand("/time-budget")
	if cmd == nil {
		t.Fatal("missing inspection worker")
	}
	msg = cmd().(sessionChangedMsg)
	if msg.err != nil || !strings.Contains(strings.Join(msg.lines, "\n"), "Deadline:") {
		t.Fatalf("status %+v", msg)
	}
	m.Update(msg)
	after, err := m.controller.TimeBudget(context.Background())
	if err != nil || !after.Deadline.Equal(before.Deadline) {
		t.Fatal("inspection renewed allowance")
	}
	text := strings.Join(timeBudgetLines(app.TimeBudgetView{Configured: true, Expired: true}), "\n")
	if !strings.Contains(text, "expired") || !strings.Contains(text, "explicit new allowance") {
		t.Fatal(text)
	}
	if m.sessionChanging {
		t.Fatal("worker not joined")
	}
}
