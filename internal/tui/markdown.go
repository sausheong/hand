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

	r, err := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(width))
	if err != nil {
		return text
	}
	out, err := r.Render(text)
	if err != nil {
		return text
	}
	return strings.TrimRight(out, "\n")
}
