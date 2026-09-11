package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"strings"
	"testing"
)

func TestFooterSeparatedAndFitsWithLongStatus(t *testing.T) {
	for _, width := range []int{40, 80, 140} {
		m := NewModel(nil, t.TempDir())
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		m.model = strings.Repeat("long-model-", 8)
		m.appendNotice("RESULT END", "toolCallStyle")
		m.textarea.SetValue("my draft")
		m.refreshViewport()
		view := m.View()
		if !strings.Contains(view, strings.Repeat("─", width)) {
			t.Fatal("missing separator")
		}
		if lipgloss.Height(view) > 30 {
			t.Fatalf("width=%d height=%d", width, lipgloss.Height(view))
		}
		if !strings.Contains(view, "my draft") {
			t.Fatal("draft hidden")
		}
		m.CloseApplication()
	}
}
