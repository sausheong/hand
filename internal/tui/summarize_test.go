package tui

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSummarizeToolCall(t *testing.T) {
	cases := []struct {
		name  string
		tool  string
		input string
		want  string
	}{
		{"read_file shows path", "read_file", `{"path":"internal/tui/model.go"}`, "internal/tui/model.go"},
		{"write_file shows path", "write_file", `{"path":"foo.go","content":"package foo"}`, "foo.go"},
		{"edit_file shows path", "edit_file", `{"path":"foo.go","old_string":"a","new_string":"b"}`, "foo.go"},
		{"bash shows command", "bash", `{"command":"go test ./..."}`, "go test ./..."},
		{"web_fetch shows url", "web_fetch", `{"url":"https://example.com"}`, "https://example.com"},
		{"web_search shows quoted query", "web_search", `{"query":"golang wordwrap"}`, `"golang wordwrap"`},
		{"unrecognized tool yields empty", "todo_write", `{"items":[]}`, ""},
		{"unparseable input yields empty", "read_file", `not json`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := summarizeToolCall(tc.tool, json.RawMessage(tc.input))
			if got != tc.want {
				t.Fatalf("summarizeToolCall(%q, %q) = %q, want %q", tc.tool, tc.input, got, tc.want)
			}
		})
	}
}

func TestTruncateOneLine(t *testing.T) {
	if got := truncateOneLine("short", 100); got != "short" {
		t.Fatalf("short string should pass through unchanged, got %q", got)
	}
	if got := truncateOneLine("line one\nline two", 100); got != "line one…" {
		t.Fatalf("multi-line input should cut at the first newline, got %q", got)
	}
	if got := truncateOneLine(strings.Repeat("a", 10), 5); got != "aaaaa…" {
		t.Fatalf("over-length input should cut at max runes, got %q", got)
	}
}

func TestSummarizeToolResult(t *testing.T) {
	if got := summarizeToolResult(""); got != "" {
		t.Fatalf("empty output should yield no snippet, got %q", got)
	}
	if got := summarizeToolResult("\n\n"); got != "" {
		t.Fatalf("whitespace-only output should yield no snippet, got %q", got)
	}

	got := summarizeToolResult("line1\nline2")
	want := "    line1\n    line2"
	if got != want {
		t.Fatalf("summarizeToolResult short output = %q, want %q", got, want)
	}

	manyLines := strings.Repeat("x\n", toolResultMaxLines+5)
	got = summarizeToolResult(manyLines)
	if !strings.Contains(got, "(truncated)") {
		t.Fatalf("output over %d lines should be marked truncated, got %q", toolResultMaxLines, got)
	}
	if n := strings.Count(got, "\n"); n > toolResultMaxLines+1 {
		t.Fatalf("truncated output has %d newlines, want at most %d", n, toolResultMaxLines+1)
	}

	longLine := strings.Repeat("y", toolResultMaxChars+50)
	got = summarizeToolResult(longLine)
	if !strings.Contains(got, "(truncated)") {
		t.Fatalf("output over %d chars should be marked truncated", toolResultMaxChars)
	}
}
