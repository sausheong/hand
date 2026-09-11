package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"
)

func TestTimingCommandDoesNotStartModel(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	defer m.CloseApplication()
	for _, running := range []bool{false, true} {
		m.running = running
		if cmd := m.handleCommand("/timing"); cmd != nil {
			t.Fatal("timing started asynchronous work")
		}
		if !strings.Contains(strings.Join(m.transcript, "\n"), "No turn timings yet.") {
			t.Fatal(m.transcript)
		}
	}
}

func TestTimingCanBeSubmittedDuringTurn(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	defer m.CloseApplication()
	m.running = true
	m.textarea.SetValue("/timing")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.textarea.Value() != "" || !strings.Contains(strings.Join(m.transcript, "\n"), "No turn timings yet.") {
		t.Fatal("running turn swallowed timing command")
	}
}
