package tui

import (
	"strings"
	"testing"
)

func TestUnknownUsageStillShowsKnownCapacity(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	defer m.CloseApplication()
	m.SetContextLimit(1000000)
	text := m.contextSummary()
	if !strings.Contains(text, "usage unknown") || !strings.Contains(text, "1M limit") || strings.Contains(text, "0/1M") {
		t.Fatal(text)
	}
}

func TestCompactionSkipIsReadable(t *testing.T) {
	for _, reason := range []string{"too_short", "cancelled", "unknown", "future_skip_reason"} {
		text := compactionSkipMessage(reason)
		if text == "" || strings.Contains(text, "_") {
			t.Fatal(text)
		}
	}
}
