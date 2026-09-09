package tui

import (
	"github.com/sausheong/harness/session"
	"strings"
	"testing"
)

func TestReplayToolOutcomeFlagsPreserved(t *testing.T) {
	for _, tc := range []struct {
		name string
		data session.ToolResultData
		want string
	}{
		{"failed-without-message", session.ToolResultData{IsError: true}, "tool failed"},
		{"aborted-without-message", session.ToolResultData{Aborted: true}, "tool cancelled"},
		{"aborted-and-failed", session.ToolResultData{Aborted: true, IsError: true}, "tool cancelled"},
		{"explicit-reason", session.ToolResultData{Aborted: true, IsError: true, Error: "deadline exceeded"}, "deadline exceeded"},
		{"error-text-only", session.ToolResultData{Error: "permission denied"}, "permission denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.data.ToolCallID = "tool-1"
			tc.data.Output = "partial diagnostic output"
			entry := session.SessionEntry{Type: session.EntryTypeToolResult, Data: mustMarshal(t, tc.data)}
			lines := ReplayHistory([]session.SessionEntry{entry}, 80, "dark")
			if len(lines) != 1 || !strings.Contains(lines[0], tc.want) || strings.Contains(lines[0], "✓") {
				t.Fatal("unsuccessful outcome rendered as success", lines)
			}
			m := NewModel(nil, t.TempDir())
			m.LoadHistory([]session.SessionEntry{entry})
			if len(m.toolOutputs) != 1 {
				t.Fatal("output not retained")
			}
			output := m.toolOutputs[0]
			if output.ToolID != "tool-1" || !strings.Contains(output.text(), tc.want) || !strings.Contains(output.text(), "partial diagnostic output") {
				t.Fatal("viewer lost outcome or diagnostic", output)
			}
			m.resize(32, 24)
			if !strings.Contains(m.transcript[0], tc.want) || strings.Contains(m.transcript[0], "✓") {
				t.Fatal("resize lost outcome", m.transcript)
			}
		})
	}
}
