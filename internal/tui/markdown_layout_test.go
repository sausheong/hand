package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"testing"
)

func markdownFixture(t *testing.T) *Model {
	t.Helper()
	m := NewModel(nil, t.TempDir())
	for i := 0; i < 8; i++ {
		m.appendSourceBlock(TranscriptBlock{Kind: "assistant", Text: fmt.Sprintf("## Block %d\n\nSome **Markdown** source with enough words to wrap at narrower widths.", i)})
	}
	m.refreshViewport()
	t.Cleanup(m.CloseApplication)
	return m
}
func TestMarkdownResizeLatestWidthWinsAndInputRemainsEditable(t *testing.T) {
	m := markdownFixture(t)
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 40, Height: 24})
	if cmd == nil || m.markdownLayout == nil {
		t.Fatal("large history did not schedule layout")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("editable")})
	if m.textarea.Value() != "editable" {
		t.Fatal("layout blocked input state")
	}
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 24})
	for cmd != nil {
		_, cmd = m.Update(cmd())
	}
	if m.markdownLayout != nil {
		t.Fatal("layout worker retained")
	}
	for _, source := range m.sourceBlocks {
		if source.Width != 30 || source.Rendered != source.Block.render(30, m.markdownStyle) {
			t.Fatal("stale resize result applied")
		}
	}
}
func TestMarkdownLayoutCannotRestoreClearedHistory(t *testing.T) {
	m := markdownFixture(t)
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 35, Height: 24})
	task := m.markdownLayout
	m.clearTranscript()
	m.appendNotice("new history", "toolCallStyle")
	m.Update(cmd())
	if len(m.sourceBlocks) != 1 || m.sourceBlocks[0].Block.Text != "new history" {
		t.Fatal("stale layout restored old history")
	}
	select {
	case <-task.done:
	default:
		t.Fatal("result published before join")
	}
}
func TestMarkdownLayoutShutdownJoins(t *testing.T) {
	m := markdownFixture(t)
	m.Update(tea.WindowSizeMsg{Width: 35, Height: 24})
	task := m.markdownLayout
	m.CloseApplication()
	if m.markdownLayout != nil {
		t.Fatal("shutdown retained task")
	}
	select {
	case <-task.done:
	default:
		t.Fatal("shutdown did not join layout")
	}
}
