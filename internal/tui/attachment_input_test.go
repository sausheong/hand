package tui

import "testing"

func TestAttachmentErrorPreservesInputWithoutStartingGoal(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.textarea.SetValue(`review "missing image.png"`)
	if cmd := m.startRun(m.textarea.Value()); cmd != nil || m.running || m.goalActive {
		t.Fatal("failed attachment started goal")
	}
	if m.textarea.Value() != `review "missing image.png"` {
		t.Fatal("attachment failure lost draft")
	}
}
