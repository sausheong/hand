package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func TestWheelScrollsLongListingAndPreservesPosition(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	defer m.CloseApplication()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.appendNotice(strings.Repeat("Skill description with its source path\n", 100), "toolCallStyle")
	m.refreshViewport()
	m.textarea.SetValue("keep my draft")
	bottom := m.viewport.YOffset
	m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	if m.viewport.YOffset != bottom-3 {
		t.Fatalf("wheel did not scroll: %d -> %d", bottom, m.viewport.YOffset)
	}
	position := m.viewport.YOffset
	m.appendNotice("More output", "toolCallStyle")
	m.refreshViewport()
	if m.viewport.YOffset != position {
		t.Fatal("new output reset scroll position")
	}
	m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if m.viewport.YOffset != position+3 || m.textarea.Value() != "keep my draft" {
		t.Fatal("wheel failed or changed input")
	}
	for range 200 {
		m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	}
	if m.viewport.YOffset != 0 {
		t.Fatal("top bound")
	}
	for range 200 {
		m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	}
	if !m.viewport.AtBottom() {
		t.Fatal("bottom bound")
	}
}

func TestMouseSelectionModeAndClicks(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	defer m.CloseApplication()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.appendNotice(strings.Repeat("line\n", 100), "toolCallStyle")
	m.refreshViewport()
	if cmd := m.handleCommand("/mouse off"); cmd == nil || !m.mouseDisabled {
		t.Fatal("mouse off failed")
	}
	position := m.viewport.YOffset
	m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	if m.viewport.YOffset != position {
		t.Fatal("disabled wheel moved transcript")
	}
	if cmd := m.handleCommand("/mouse on"); cmd == nil || m.mouseDisabled {
		t.Fatal("mouse on failed")
	}
	position = m.viewport.YOffset
	m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.viewport.YOffset != position || m.running {
		t.Fatal("click changed state")
	}
}

func TestWheelTargetsOutputViewer(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	defer m.CloseApplication()
	v := viewport.New(80, 10)
	v.SetContent(strings.Repeat("output\n", 100))
	v.GotoBottom()
	m.outputView = &outputViewer{viewport: v}
	before := m.outputView.viewport.YOffset
	m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	if m.outputView.viewport.YOffset >= before || m.viewport.YOffset != 0 {
		t.Fatal("wheel did not target output viewer")
	}
}
