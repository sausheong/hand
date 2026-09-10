package tui

import (
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
	if block, ok := sessionBlock(entry); ok {
		return block.render(width, style), true
	}
	return "", false
}

// LoadHistory appends the replayed lines from entries to the transcript
// and refreshes the viewport. Call once, right after NewApplicationModel, when
// resuming a session with non-empty History().
func (m *Model) LoadHistory(entries []session.SessionEntry) {
	for _, entry := range entries {
		if block, ok := outputFromEntry(entry); ok {
			block.SessionID = m.identity.SessionID
			m.toolOutputs = append(m.toolOutputs, block)
		}
		if block, ok := sessionBlock(entry); ok {
			m.appendSourceBlock(block)
		} else if line, ok := replayEntry(entry, m.termWidth, m.markdownStyle); ok {
			m.appendNotice(sanitizeForTerminal(line), "errorLineStyle")
		}
	}
	m.refreshViewport()
}
