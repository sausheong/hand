package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sausheong/harness/session"
)

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	return data
}

func TestReplayHistory_UserMessage(t *testing.T) {
	entries := []session.SessionEntry{
		{Type: session.EntryTypeMessage, Role: "user", Data: mustMarshal(t, session.MessageData{Text: "hello there"})},
	}
	lines := ReplayHistory(entries, 80)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "> hello there") {
		t.Fatalf("line = %q, want it to contain \"> hello there\"", lines[0])
	}
}

func TestReplayHistory_AssistantMessage(t *testing.T) {
	entries := []session.SessionEntry{
		{Type: session.EntryTypeMessage, Role: "assistant", Data: mustMarshal(t, session.MessageData{Text: "hi back"})},
	}
	lines := ReplayHistory(entries, 80)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "hi back") {
		t.Fatalf("line = %q, want it to contain \"hi back\"", lines[0])
	}
}

func TestReplayHistory_ToolCall(t *testing.T) {
	entries := []session.SessionEntry{
		{Type: session.EntryTypeToolCall, Data: mustMarshal(t, session.ToolCallData{Tool: "read_file", ID: "tc1", Input: json.RawMessage(`{}`)})},
	}
	lines := ReplayHistory(entries, 80)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "[tool: read_file]") {
		t.Fatalf("line = %q, want it to contain \"[tool: read_file]\"", lines[0])
	}
}

func TestReplayHistory_ToolResultSuccess(t *testing.T) {
	entries := []session.SessionEntry{
		{Type: session.EntryTypeToolResult, Data: mustMarshal(t, session.ToolResultData{ToolCallID: "tc1", Output: "ok"})},
	}
	lines := ReplayHistory(entries, 80)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "✓") {
		t.Fatalf("line = %q, want it to contain \"✓\"", lines[0])
	}
}

func TestReplayHistory_ToolResultFailure(t *testing.T) {
	entries := []session.SessionEntry{
		{Type: session.EntryTypeToolResult, Data: mustMarshal(t, session.ToolResultData{ToolCallID: "tc1", Error: "file not found", IsError: true})},
	}
	lines := ReplayHistory(entries, 80)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "✗") || !strings.Contains(lines[0], "file not found") {
		t.Fatalf("line = %q, want it to contain \"✗\" and \"file not found\"", lines[0])
	}
}

func TestReplayHistory_Compaction(t *testing.T) {
	entries := []session.SessionEntry{
		{Type: session.EntryTypeCompaction, Data: mustMarshal(t, session.CompactionData{Summary: "old stuff", TurnsCompacted: 12})},
	}
	lines := ReplayHistory(entries, 80)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "12") {
		t.Fatalf("line = %q, want it to mention the compacted turn count (12)", lines[0])
	}
}

func TestReplayHistory_Meta(t *testing.T) {
	entries := []session.SessionEntry{
		{Type: session.EntryTypeMeta, Role: "system", Data: mustMarshal(t, session.MessageData{Text: "a system note"})},
	}
	lines := ReplayHistory(entries, 80)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "a system note") {
		t.Fatalf("line = %q, want it to contain \"a system note\"", lines[0])
	}
}

func TestReplayHistory_MultipleEntriesPreserveOrder(t *testing.T) {
	entries := []session.SessionEntry{
		{Type: session.EntryTypeMessage, Role: "user", Data: mustMarshal(t, session.MessageData{Text: "first"})},
		{Type: session.EntryTypeMessage, Role: "assistant", Data: mustMarshal(t, session.MessageData{Text: "second"})},
	}
	lines := ReplayHistory(entries, 80)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "first") || !strings.Contains(lines[1], "second") {
		t.Fatalf("lines = %v, want order preserved (first, then second)", lines)
	}
}

func TestModel_LoadHistory_PopulatesTranscript(t *testing.T) {
	m := NewModel(&fakeRunner{}, t.TempDir())
	m.LoadHistory([]session.SessionEntry{
		{Type: session.EntryTypeMessage, Role: "user", Data: mustMarshal(t, session.MessageData{Text: "resumed message"})},
	})
	found := false
	for _, line := range m.transcript {
		if strings.Contains(line, "resumed message") {
			found = true
		}
	}
	if !found {
		t.Fatalf("transcript = %v, want a line containing \"resumed message\"", m.transcript)
	}
}
