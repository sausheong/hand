package tui

import (
	"strings"
	"testing"
)

func TestStartupDraftTransferPreservesLongInput(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	text := strings.Repeat("draft ", 1000)
	m.SetInputDraft(text)
	if m.textarea.Value() != text {
		t.Fatal("startup draft truncated during handover")
	}
}
