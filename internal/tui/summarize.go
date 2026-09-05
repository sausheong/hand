package tui

import (
	"encoding/json"
	"fmt"
	"strings"
)

// summaryMaxLen caps how much of a tool call's input (a path, command,
// URL, or query) shows on its "[tool: name] ..." transcript line — long
// enough to be useful, short enough that a multi-line heredoc or giant
// path doesn't blow up the header line. The full command for a gated
// tool (bash, write_file, edit_file) is already shown in full on the
// approval prompt; this is just the at-a-glance summary.
const summaryMaxLen = 100

// summarizeToolCall renders a short, human-readable detail for a tool
// call's input — which file, which command, which URL or query — so the
// transcript reads as "[tool: read_file] internal/tui/model.go" instead
// of a bare tool name. Unrecognized tools or unparseable input yield ""
// (falls back to the bare name), never an error.
func summarizeToolCall(name string, input json.RawMessage) string {
	var field string
	switch name {
	case "read_file", "write_file", "edit_file":
		var in struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(input, &in) == nil {
			field = in.Path
		}
	case "bash":
		var in struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(input, &in) == nil {
			field = in.Command
		}
	case "web_fetch":
		var in struct {
			URL string `json:"url"`
		}
		if json.Unmarshal(input, &in) == nil {
			field = in.URL
		}
	case "web_search":
		var in struct {
			Query string `json:"query"`
		}
		if json.Unmarshal(input, &in) == nil && in.Query != "" {
			field = fmt.Sprintf("%q", in.Query)
		}
	}
	return truncateOneLine(field, summaryMaxLen)
}

// truncateOneLine collapses s to its first line and caps it at max
// runes, marking either cut with "…".
func truncateOneLine(s string, max int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i] + "…"
	}
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "…"
	}
	return s
}

// toolResultMaxLines and toolResultMaxChars cap the tool-result snippet
// shown under the ✓ in the transcript — a preview of what the tool
// actually returned, not the full (possibly huge) output.
const (
	toolResultMaxLines = 6
	toolResultMaxChars = 500
)

// summarizeToolResult renders an indented preview of a successful tool
// call's output, or "" for empty output (just the ✓ shows, nothing
// below it).
func summarizeToolResult(output string) string {
	output = strings.TrimRight(output, "\n")
	if output == "" {
		return ""
	}

	lines := strings.Split(output, "\n")
	truncated := false
	if len(lines) > toolResultMaxLines {
		lines = lines[:toolResultMaxLines]
		truncated = true
	}
	snippet := strings.Join(lines, "\n")
	if len(snippet) > toolResultMaxChars {
		snippet = snippet[:toolResultMaxChars]
		truncated = true
	}
	if truncated {
		snippet += "\n… (truncated)"
	}

	indented := strings.Split(snippet, "\n")
	for i, line := range indented {
		indented[i] = "    " + line
	}
	return strings.Join(indented, "\n")
}
