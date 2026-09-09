package tui

import "testing"

func TestReloadCommandGuardsAndJoins(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	if cmd := m.handleCommand("/reload unexpected"); cmd != nil {
		t.Fatal("invalid reload dispatched")
	}
	m.running = true
	if cmd := m.handleCommand("/reload"); cmd != nil {
		t.Fatal("busy reload dispatched")
	}
	m.running = false
	cmd := m.handleCommand("/reload")
	if cmd == nil || !m.sessionChanging {
		t.Fatal("reload did not enter joined operation")
	}
	m.Update(cmd())
	if m.sessionChanging {
		t.Fatal("reload operation did not finish")
	}
}
