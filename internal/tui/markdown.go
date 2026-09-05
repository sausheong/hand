package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

// minMarkdownWidth is the floor passed to glamour.WithWordWrap —
// glamour's block/table layout misbehaves on very narrow widths, and a
// terminal narrower than this is already unusable for much else.
const minMarkdownWidth = 20

// renderMarkdown renders text (assistant output, which models format as
// Markdown — headers, bold/italic, lists, code fences, tables) into
// ANSI-styled terminal output wrapped to width columns.
//
// Deliberately glamour.WithStandardStyle("dark"), never the automatic
// style-detection option: detecting light vs. dark means querying the
// terminal for its background color over stdin/stdout — an OSC 11
// query, answered with an "rgb:RRRR/GGGG/BBBB" response on the same raw
// stdin Bubble Tea's own input loop is concurrently reading. The two
// readers race for those bytes; when Bubble Tea's loop wins, the
// terminal's raw response is parsed as literal keystrokes and dumped
// straight into the input box (this is exactly what shipped, and
// exactly what a user hit — bytes like "rgb:0000/0000/0000]11;..."
// appearing in the input after a Markdown response rendered). A fixed
// style never sends that query, so this class of corruption can't
// happen. hand's own lipgloss palette (styles.go) is likewise a fixed,
// non-adaptive dark theme, so this matches how the rest of the UI
// already looks. See TestRenderMarkdown_NeverUsesAutoStyle.
//
// A fresh glamour.TermRenderer is built on every call rather than
// reused: renderMarkdown only runs at flush points (Model.flushStream —
// once per tool-call boundary, once at turn end — and session replay),
// not per streamed character, so the cost of loading glamour's style
// definitions is negligible next to an LLM round trip. Building fresh
// also means the wrap width always matches the terminal's current size
// instead of one cached from an earlier, possibly since-resized width.
//
// Falls back to text unchanged on any error (a renderer that fails to
// construct, or Markdown glamour can't parse) — a rendering problem
// must never make an assistant response disappear from the transcript.
func renderMarkdown(text string, width int) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	if width < minMarkdownWidth {
		width = minMarkdownWidth
	}

	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(width))
	if err != nil {
		return text
	}
	out, err := r.Render(text)
	if err != nil {
		return text
	}
	return strings.TrimRight(out, "\n")
}
