package tui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ansiEscapePattern matches ANSI/terminal escape sequences: CSI (cursor
// movement, colors, screen clears), OSC (window title, clipboard writes
// via OSC 52, hyperlinks — terminated by BEL or ST), and the shorter
// single-character Fe/Fp/Fs escapes. Content flowing through here
// (tool call summaries, tool output previews, streamed model text,
// replayed session history) can originate from an untrusted web page,
// file, or MCP server — sanitizeForTerminal strips anything that could
// reprogram the user's terminal before it reaches lipgloss/bubbletea.
var ansiEscapePattern = regexp.MustCompile(
	"\x1b(?:" +
		`\][^\x07\x1b]*(?:\x07|\x1b\\)` + // OSC ... BEL or ST
		`|\[[0-9;?]*[ -/]*[@-~]` + // CSI
		`|[@-Z\\\]^_]` + // Fe/Fp/Fs single-char escapes
		")",
)

// sanitizeForTerminal strips ANSI escape sequences and other C0 control
// bytes (keeping \n and \t, which are just formatting) from s. See
// ansiEscapePattern for what this defends against and why.
func sanitizeForTerminal(s string) string {
	s = ansiEscapePattern.ReplaceAllString(s, "")

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\t' {
			b.WriteRune(r)
			continue
		}
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

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
	return truncateOneLine(sanitizeForTerminal(field), summaryMaxLen)
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
// actually returned, not the full (possibly huge) output. Generous
// enough that a typical `go test`/`git diff`/build-error output shows
// in full without immediately needing to scroll back for it (full
// history is still just a pgup/pgdown or mouse-wheel scroll away —
// see the viewport scroll handling in handleKey/Update); still capped
// so a single command dumping megabytes of output can't blow up
// rendering or memory.
const (
	toolResultMaxLines = 30
	toolResultMaxChars = 4000
)

// summarizeToolResult renders an indented preview of a successful tool
// call's output, or "" for empty output (just the ✓ shows, nothing
// below it).
func summarizeToolResult(output string) string {
	output = sanitizeForTerminal(strings.TrimRight(output, "\n"))
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
