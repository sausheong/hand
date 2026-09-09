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
	lines := ReplayHistory(entries, 80, "dark")
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
	lines := ReplayHistory(entries, 80, "dark")
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %v", len(lines), lines)
	}
	// Rendered as Markdown (renderMarkdown), which — with a fixed style,
	// see renderMarkdown's own comment on why never WithAutoStyle —
	// always applies real ANSI codes, including at word-wrap boundaries;
	// strip them before checking content survived.
	if got := sanitizeForTerminal(lines[0]); !strings.Contains(got, "hi back") {
		t.Fatalf("line (ANSI stripped) = %q, want it to contain \"hi back\"", got)
	}
}

func TestReplayHistory_ToolCall(t *testing.T) {
	entries := []session.SessionEntry{
		{Type: session.EntryTypeToolCall, Data: mustMarshal(t, session.ToolCallData{Tool: "read_file", ID: "tc1", Input: json.RawMessage(`{}`)})},
	}
	lines := ReplayHistory(entries, 80, "dark")
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
	lines := ReplayHistory(entries, 80, "dark")
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
	lines := ReplayHistory(entries, 80, "dark")
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "✗") || !strings.Contains(lines[0], "file not found") {
		t.Fatalf("line = %q, want it to contain \"✗\" and \"file not found\"", lines[0])
	}
}

// Regression: a replayed tool name/error is exactly as untrusted as the
// live path's (an MCP server's own tool name, bash stderr, etc.) — same
// sanitization requirement, same bug class as the live-path fix.
func TestReplayHistory_SanitizesToolNameAndError(t *testing.T) {
	entries := []session.SessionEntry{
		{Type: session.EntryTypeToolCall, Data: mustMarshal(t, session.ToolCallData{Tool: "mcp__evil__\x1b]0;pwned\x07tool", ID: "tc1", Input: json.RawMessage(`{}`)})},
		{Type: session.EntryTypeToolResult, Data: mustMarshal(t, session.ToolResultData{ToolCallID: "tc2", Error: "failed\x1b]52;c;ZXZpbA==\x07", IsError: true})},
	}
	lines := ReplayHistory(entries, 80, "dark")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %v", len(lines), lines)
	}
	for _, line := range lines {
		if strings.ContainsRune(line, '\x1b') || strings.ContainsRune(line, '\x07') {
			t.Fatalf("line = %q, still contains raw ANSI/control bytes", line)
		}
	}
	if !strings.Contains(lines[1], "failed") {
		t.Fatalf("line = %q, lost the legitimate error text", lines[1])
	}
}

func TestReplayHistory_Compaction(t *testing.T) {
	entries := []session.SessionEntry{
		{Type: session.EntryTypeCompaction, Data: mustMarshal(t, session.CompactionData{Summary: "old stuff", TurnsCompacted: 12})},
	}
	lines := ReplayHistory(entries, 80, "dark")
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
	lines := ReplayHistory(entries, 80, "dark")
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
	lines := ReplayHistory(entries, 80, "dark")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "first") || !strings.Contains(lines[1], "second") {
		t.Fatalf("lines = %v, want order preserved (first, then second)", lines)
	}
}

func TestModel_LoadHistory_PopulatesTranscript(t *testing.T) {
	m := NewModel(nil, t.TempDir())
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

func TestReplayMalformedHistoryRemainsVisibleAndOrdered(t *testing.T) {
	cases := []struct {
		kind  session.EntryType
		label string
	}{
		{session.EntryTypeMessage, "message"},
		{session.EntryTypeToolCall, "tool call"},
		{session.EntryTypeToolResult, "tool result"},
		{session.EntryTypeCompaction, "compaction"},
		{session.EntryTypeMeta, "note"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			for _, raw := range []string{`null`, `{`, `[]`, `{"text":123,"tool":123,"output":123,"summary":123}`, "{\x1b]52;c;attack\x07"} {
				entries := []session.SessionEntry{session.UserMessageEntry("before"), {Type: tc.kind, Data: json.RawMessage(raw)}, session.UserMessageEntry("after")}
				lines := ReplayHistory(entries, 80, "dark")
				if len(lines) != 3 || !strings.Contains(lines[0], "before") || !strings.Contains(lines[2], "after") || !strings.Contains(lines[1], "could not replay "+tc.label) {
					t.Fatalf("malformed history hidden or reordered: %q", lines)
				}
				m := NewModel(nil, t.TempDir())
				m.LoadHistory(entries)
				if len(m.toolOutputs) != 0 {
					t.Fatal("malformed result became an inspectable successful output")
				}
				if len(m.transcript) != 3 || !strings.Contains(m.transcript[1], "could not replay "+tc.label) {
					t.Fatal("model lost error", m.transcript)
				}
				m.termWidth = 42
				m.refreshSourceBlocks()
				if !strings.Contains(m.transcript[1], "could not replay "+tc.label) {
					t.Fatal("resize lost replay error")
				}
				if strings.Contains(m.transcript[1], "\x1b]52;") || strings.ContainsRune(m.transcript[1], '\x07') {
					t.Fatal("replay error emitted terminal control")
				}
			}
		})
	}
	unknown := session.SessionEntry{Type: "future-entry", Data: json.RawMessage(`{}`)}
	if lines := ReplayHistory([]session.SessionEntry{unknown}, 80, "dark"); len(lines) != 0 {
		t.Fatal("unknown entry rendered", lines)
	}
}
