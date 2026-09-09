package tui

import (
	"context"
	"strings"
	"testing"
)

func TestStructuredStateTerminalOperations(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	for _, command := range []string{"/state set", "/state remove", "/state evidence id ref", "/state set id invalid text", "/state extra"} {
		if cmd := m.handleCommand(command); cmd != nil {
			t.Fatal("invalid command dispatched", command)
		}
	}
	execute := func(command string) sessionChangedMsg {
		t.Helper()
		cmd := m.handleCommand(command)
		if cmd == nil {
			t.Fatal("command not dispatched", command)
		}
		msg := cmd().(sessionChangedMsg)
		m.Update(msg)
		if msg.err != nil {
			t.Fatal(msg.err)
		}
		if m.sessionChanging {
			t.Fatal("operation not joined")
		}
		return msg
	}
	execute("/state set goal objective Deliver the migration")
	execute("/state set choice decision Preserve old config")
	execute("/state set todo unresolved_work Test rollback")
	execute("/state evidence tests evidence/run.json#abc Unit tests at snapshot abc")
	execute("/state set choice decision Preserve old and new config")
	state, err := m.controller.ContextState(context.Background())
	if err != nil || len(state.Items) != 4 || state.Revision != 5 {
		t.Fatal(state, err)
	}
	text := strings.Join(execute("/state").lines, "\n")
	for _, part := range []string{"revision 5", "Preserve old and new config", "evidence/run.json#abc", "do not establish current verification"} {
		if !strings.Contains(text, part) {
			t.Fatal(text)
		}
	}
	execute("/state remove todo")
	state, err = m.controller.ContextState(context.Background())
	if err != nil || len(state.Items) != 3 || state.Revision != 6 {
		t.Fatal(state, err)
	}
	m.running = true
	if cmd := m.handleCommand("/state remove goal"); cmd != nil {
		t.Fatal("busy mutation dispatched")
	}
	m.running = false
	cmd := m.handleCommand("/state remove missing")
	if cmd == nil {
		t.Fatal("missing worker")
	}
	msg := cmd().(sessionChangedMsg)
	m.Update(msg)
	if msg.err == nil {
		t.Fatal("missing removal accepted")
	}
	state, err = m.controller.ContextState(context.Background())
	if err != nil || len(state.Items) != 3 || state.Revision != 6 {
		t.Fatal("failed mutation changed state", state, err)
	}
}
