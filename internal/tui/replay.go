package tui

import (
	"encoding/json"
	"fmt"

	"github.com/sausheong/harness/session"
)

// ReplayHistory turns stored session entries back into the same kind of
// styled lines the live event path produces, so a resumed session shows
// prior conversation instead of a blank transcript. width sets the
// Markdown wrap column and style the glamour style (see renderMarkdown)
// for replayed assistant messages — pass the model's current termWidth
// and markdownStyle. Malformed entry data (should not happen for
// entries this program wrote itself) renders as a visible placeholder
// rather than being silently dropped or panicking.
func ReplayHistory(entries []session.SessionEntry, width int, style string) []string {
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		if line, ok := replayEntry(entry, width, style); ok {
			lines = append(lines, line)
		}
	}
	return lines
}

func replayEntry(entry session.SessionEntry, width int, style string) (string, bool) {
	switch entry.Type {
	case session.EntryTypeMessage:
		var data session.MessageData
		if err := json.Unmarshal(entry.Data, &data); err != nil {
			return toolErrStyle.Render("  ✗ could not replay message: " + err.Error()), true
		}
		if entry.Role == "user" {
			return userLineStyle.Render("> " + sanitizeForTerminal(data.Text)), true
		}
		// assistant text renders as Markdown, matching the live
		// streamed-and-flushed path (see Model.flushStream).
		return renderMarkdown(sanitizeForTerminal(data.Text), width, style), true

	case session.EntryTypeToolCall:
		var data session.ToolCallData
		if err := json.Unmarshal(entry.Data, &data); err != nil {
			return toolErrStyle.Render("  ✗ could not replay tool call: " + err.Error()), true
		}
		// data.Tool is an MCP server's own registered tool name, not
		// necessarily one of hand's fixed built-ins — same sanitization
		// as the live path (see Model.handleAgentEvent's EventToolCallStart).
		return toolCallStyle.Render(fmt.Sprintf("[tool: %s]", sanitizeForTerminal(data.Tool))), true

	case session.EntryTypeToolResult:
		var data session.ToolResultData
		if err := json.Unmarshal(entry.Data, &data); err != nil {
			return toolErrStyle.Render("  ✗ could not replay tool result: " + err.Error()), true
		}
		if data.Error != "" {
			// Same untrusted-content reasoning as the live path's
			// EventToolResult error branch — sanitize before rendering.
			return toolErrStyle.Render("  ✗ " + sanitizeForTerminal(data.Error)), true
		}
		return toolOKStyle.Render("  ✓"), true

	case session.EntryTypeCompaction:
		var data session.CompactionData
		if err := json.Unmarshal(entry.Data, &data); err != nil {
			return toolErrStyle.Render("  ✗ could not replay compaction: " + err.Error()), true
		}
		return toolCallStyle.Render(fmt.Sprintf("[compacted %d turns]", data.TurnsCompacted)), true

	case session.EntryTypeMeta:
		var data session.MessageData
		if err := json.Unmarshal(entry.Data, &data); err != nil {
			return toolErrStyle.Render("  ✗ could not replay note: " + err.Error()), true
		}
		return toolCallStyle.Render("[" + sanitizeForTerminal(data.Text) + "]"), true

	default:
		return "", false
	}
}

// LoadHistory appends the replayed lines from entries to the transcript
// and refreshes the viewport. Call once, right after NewModel, when
// resuming a session with non-empty History().
func (m *Model) LoadHistory(entries []session.SessionEntry) {
	m.transcript = append(m.transcript, ReplayHistory(entries, m.termWidth, m.markdownStyle)...)
	m.refreshViewport()
}
