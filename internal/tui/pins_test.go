package tui

import "testing"

func TestPinCommandsUseJoinedOperations(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	for _, command := range []string{"/pin", "/pin id objective", "/unpin", "/pins extra"} {
		if cmd := m.handleCommand(command); cmd != nil {
			t.Fatal("invalid command dispatched", command)
		}
	}
	m.running = true
	if cmd := m.handleCommand("/pin id objective text"); cmd != nil {
		t.Fatal("busy pin dispatched")
	}
	m.running = false
	for _, command := range []string{"/pin id objective Keep this objective", "/pins", "/unpin id"} {
		cmd := m.handleCommand(command)
		if cmd == nil {
			t.Fatal("command not dispatched", command)
		}
		m.Update(cmd())
		if m.sessionChanging {
			t.Fatal("operation not joined")
		}
	}
}
