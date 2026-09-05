package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/sausheong/hand/internal/config"
)

// minMarkdownWidth is the floor passed to glamour.WithWordWrap —
// glamour's block/table layout misbehaves on very narrow widths, and a
// terminal narrower than this is already unusable for much else.
const minMarkdownWidth = 20

// safeMarkdownStyle returns style if it's one of config.ValidMarkdownStyles,
// otherwise config.DefaultMarkdownStyle. renderMarkdown's own last line
// of defense — config.ResolveMarkdownStyle already validates
// --markdown-style/config.json's markdown_style before it ever reaches
// here, but this function is the one actually handing a style string to
// glamour, so it enforces the same allow-list itself rather than
// trusting every caller got that right. Every name on the list renders
// with zero terminal I/O; deliberately never "auto" — see this file's
// package comment above renderMarkdown for why letting that through
// would matter, not just look wrong.
func safeMarkdownStyle(style string) string {
	for _, s := range config.ValidMarkdownStyles {
		if s == style {
			return style
		}
	}
	return config.DefaultMarkdownStyle
}

// renderMarkdown renders text (assistant output, which models format as
// Markdown — headers, bold/italic, lists, code fences, tables) into
// ANSI-styled terminal output wrapped to width columns, using one of
// config.ValidMarkdownStyles (see safeMarkdownStyle — an unrecognized
// style falls back to config.DefaultMarkdownStyle rather than erroring).
//
// style must always be a fixed glamour.WithStandardStyle name, never
// glamour's own "auto" detection: auto-style means querying the
// terminal for its background color over stdin/stdout — an OSC 11
// query, answered with an "rgb:RRRR/GGGG/BBBB" response on the same raw
// stdin Bubble Tea's own input loop is concurrently reading. The two
// readers race for those bytes; when Bubble Tea's loop wins, the
// terminal's raw response is parsed as literal keystrokes and dumped
// straight into the input box (this is exactly what shipped once, and
// exactly what a user hit — bytes like "rgb:0000/0000/0000]11;..."
// appearing in the input after a Markdown response rendered). A fixed
// style never sends that query, so this class of corruption can't
// happen. See TestRenderMarkdown_NeverUsesAutoStyle.
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
func renderMarkdown(text string, width int, style string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	if width < minMarkdownWidth {
		width = minMarkdownWidth
	}

	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle(safeMarkdownStyle(style)), glamour.WithWordWrap(width))
	if err != nil {
		return text
	}
	out, err := r.Render(text)
	if err != nil {
		return text
	}
	return strings.TrimRight(out, "\n")
}
