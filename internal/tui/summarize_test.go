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

// Regression: bash/web_fetch/MCP tool output is untrusted external
// content. Before sanitizeForTerminal existed, summarizeToolResult
// passed it straight into the transcript, so a malicious page or
// compromised MCP server could inject terminal escape sequences (OSC 52
// clipboard writes, title-bar spoofing, CSI cursor tricks) that render
// live in the user's terminal.
func TestSummarizeToolResult_StripsANSIEscapes(t *testing.T) {
	malicious := "before\x1b[31mred\x1b[0m\x1b]0;evil title\x07after"
	got := summarizeToolResult(malicious)
	if strings.ContainsRune(got, '\x1b') {
		t.Fatalf("summarizeToolResult output still contains ESC: %q", got)
	}
	if strings.ContainsRune(got, '\x07') {
		t.Fatalf("summarizeToolResult output still contains BEL: %q", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "red") || !strings.Contains(got, "after") {
		t.Fatalf("sanitized output lost legitimate content: %q", got)
	}
}

func TestSummarizeToolCall_StripsANSIEscapes(t *testing.T) {
	input := json.RawMessage(`{"command":"echo [31mhi[0m"}`)
	got := summarizeToolCall("bash", input)
	if strings.ContainsRune(got, '\x1b') {
		t.Fatalf("summarizeToolCall output still contains ESC: %q", got)
	}
}

func TestSanitizeForTerminal(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text passes through", "hello world", "hello world"},
		{"newline and tab preserved", "a\nb\tc", "a\nb\tc"},
		{"CSI color codes stripped", "\x1b[1;31merror\x1b[0m", "error"},
		{"OSC 52 clipboard write stripped (BEL terminator)", "\x1b]52;c;ZXZpbA==\x07after", "after"},
		{"OSC title stripped (ST terminator)", "\x1b]0;pwned\x1b\\after", "after"},
		{"bare BEL stripped", "ding\x07dong", "dingdong"},
		{"other control bytes stripped", "a\x00\x01\x7fb", "ab"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeForTerminal(tc.in); got != tc.want {
				t.Fatalf("sanitizeForTerminal(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
