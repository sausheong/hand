package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

type profilePicker struct {
	names    []string
	selected int
}

func (m *Model) openProfilePicker() {
	names := m.controller.ProfileNames()
	if len(names) == 0 {
		m.appendNotice("no named profiles configured", "toolCallStyle")
		return
	}
	m.abandonGoal("profile_selection")
	selected := 0
	for i, name := range names {
		if name == m.controller.CurrentProfile() {
			selected = i
		}
	}
	m.profilePicker = &profilePicker{names: names, selected: selected}
}

func (m *Model) handleProfilePickerKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.profilePicker
	switch key.String() {
	case "esc", "ctrl+c":
		m.profilePicker = nil
	case "up", "k":
		p.selected = (p.selected + len(p.names) - 1) % len(p.names)
	case "down", "j":
		p.selected = (p.selected + 1) % len(p.names)
	case "home":
		p.selected = 0
	case "end":
		p.selected = len(p.names) - 1
	case "enter":
		name := p.names[p.selected]
		m.profilePicker = nil
		return m, m.runProfileCommand([]string{name})
	}
	return m, nil
}

func (m *Model) profilePickerView() string {
	p := m.profilePicker
	height, width := max(1, m.termHeight), max(1, m.termWidth)
	rows := max(1, min(10, height-7))
	start := max(0, min(p.selected-rows/2, len(p.names)-rows))
	lines := []string{"Choose profile"}
	for i := start; i < min(len(p.names), start+rows); i++ {
		marker := "  "
		if i == p.selected {
			marker = "> "
		}
		name := sanitizeForTerminal(p.names[i])
		if p.names[i] == m.controller.CurrentProfile() {
			name += " (active)"
		}
		lines = append(lines, marker+name)
	}
	if profile, ok := m.controller.ProfileConfiguration(p.names[p.selected]); ok {
		context := "automatic"
		if profile.ContextLimit > 0 {
			context = fmt.Sprint(profile.ContextLimit)
		} else if profile.MetadataProtocol != "" {
			context = "server discovery"
		}
		output := "automatic"
		if profile.MaxOutput > 0 {
			output = fmt.Sprint(profile.MaxOutput)
		}
		reasoning := profile.Reasoning
		if reasoning == "" {
			reasoning = "off"
		}
		lines = append(lines, "", sanitizeForTerminal(profile.Provider+"/"+profile.Model), "Configured context: "+context+"; output: "+output, "Reasoning: "+sanitizeForTerminal(reasoning))
	}
	lines = append(lines, "↑/↓ choose · Enter switch · Esc cancel")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, width, "…")
	}
	return strings.Join(lines, "\n")
}
