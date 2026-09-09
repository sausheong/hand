package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"os"
	"path/filepath"
	"testing"
)

func TestReferenceTabCompletesAtCursorAndIgnoresStaleResult(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "note file.txt"), []byte("x"), 0600)
	m := NewModel(nil, dir)
	m.textarea.SetValue("look @no suffix")
	m.textarea.SetCursor(len("look @no"))
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	if cmd == nil {
		t.Fatal("no completion command")
	}
	msg := cmd()
	m.Update(msg)
	if got := m.textarea.Value(); got != `look @"note file.txt" suffix` {
		t.Fatal(got)
	}
	if m.inputCursor() != len(`look @"note file.txt"`) {
		t.Fatal("cursor moved into suffix", m.inputCursor())
	}
	m.textarea.SetValue("@no")
	cmd = m.completeReference()
	msg = cmd()
	m.textarea.SetValue("new draft")
	m.Update(msg)
	if m.textarea.Value() != "new draft" {
		t.Fatal("stale completion replaced input")
	}
}
