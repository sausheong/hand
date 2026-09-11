package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"strings"
)

func isLiveInspectionCommand(text string) bool {
	fields := strings.Fields(text)
	return len(fields) > 0 && (fields[0] == "/timing" || fields[0] == "/mouse")
}

func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.mouseDisabled || m.editorActive || m.profilePicker != nil {
		return m, nil
	}
	// Clicks and drags must never type text, submit a prompt or grant approval.
	if msg.Button != tea.MouseButtonWheelUp && msg.Button != tea.MouseButtonWheelDown {
		return m, nil
	}
	if m.outputView != nil {
		var cmd tea.Cmd
		m.outputView.viewport, cmd = m.outputView.viewport.Update(msg)
		return m, cmd
	}
	const rows = 3
	if msg.Button == tea.MouseButtonWheelUp {
		m.viewport.YOffset = max(0, m.viewport.YOffset-rows)
	} else {
		m.viewport.YOffset = min(m.viewport.maxOffset(), m.viewport.YOffset+rows)
	}
	return m, nil
}
