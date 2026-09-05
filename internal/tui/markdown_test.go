package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/sausheong/harness/runtime"
)

// Regression: glamour.WithAutoStyle queries the terminal for its
// background color over stdin/stdout (an interactive OSC round trip),
// racing Bubble Tea's own raw-mode stdin reader for those bytes. When
// Bubble Tea's reader wins the race, the terminal's raw response
// ("rgb:0000/0000/0000]11;...") gets parsed as literal keystrokes and
// dumped into whatever has focus — the message input box, in practice.
// This isn't something a runtime test can catch (it's a real terminal
// I/O race, not a pure-function bug), so this is a direct source guard
// instead: renderMarkdown must never reference glamour.WithAutoStyle as
// an actual call. An AST walk (not a text search) so this doesn't
// false-positive on comments that name the very thing to avoid — like
// the one on renderMarkdown itself.
func TestRenderMarkdown_NeverUsesAutoStyle(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "markdown.go", nil, 0)
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if ok && pkg.Name == "glamour" && sel.Sel.Name == "WithAutoStyle" {
			t.Fatal("markdown.go must never call glamour.WithAutoStyle — it queries the terminal for its background color and races Bubble Tea's own stdin reader for the response, which can leak into the input box as literal text; use a fixed glamour.WithStandardStyle instead")
		}
		return true
	})
}

func TestRenderMarkdown_EmptyOrWhitespaceReturnsUnchanged(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\t\n"} {
		if got := renderMarkdown(in, 80, "dark"); got != in {
			t.Errorf("renderMarkdown(%q, 80, dark) = %q, want unchanged", in, got)
		}
	}
}

// renderMarkdown now always uses a fixed style (glamour.WithStandardStyle
// "dark" — see renderMarkdown's own comment on why never WithAutoStyle),
// so real ANSI codes are always present, unlike when a terminal-queried
// auto-style could silently fall back to an unstyled "notty" style in a
// non-TTY `go test` run. Glamour's word-wrap re-opens/closes style codes
// at wrap boundaries though, so a multi-word phrase isn't necessarily
// one contiguous byte run any more — assertions strip ANSI first with
// the package's own sanitizeForTerminal before checking content.
func TestRenderMarkdown_PreservesContent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text", "just plain text", "plain text"},
		{"heading", "# A Heading", "A Heading"},
		{"bold and italic", "some **bold** and *italic* words", "bold"},
		{"list", "- item one\n- item two", "item one"},
		{"code fence", "```go\nfunc main() {}\n```", "func main"},
		{"blockquote", "> a quoted line", "a quoted line"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeForTerminal(renderMarkdown(tc.in, 80, "dark"))
			if !strings.Contains(got, tc.want) {
				t.Fatalf("renderMarkdown(%q, 80, dark) (ANSI stripped) = %q, want it to contain %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRenderMarkdown_ClampsNarrowOrInvalidWidth(t *testing.T) {
	for _, w := range []int{0, -5, 1, minMarkdownWidth - 1} {
		got := sanitizeForTerminal(renderMarkdown("hello world", w, "dark"))
		if !strings.Contains(got, "hello") {
			t.Fatalf("renderMarkdown(%q, width=%d, dark) (ANSI stripped) = %q, want it to still contain the content", "hello world", w, got)
		}
	}
}

// Regression: renderMarkdown must never hand glamour an unrecognized
// (or, worse, "auto") style — it validates the style itself
// (safeMarkdownStyle) rather than trusting the caller already did,
// since config.ResolveMarkdownStyle isn't the only path that can reach
// here. Proven by checking the output is identical to explicitly
// passing "dark", not just "didn't crash".
func TestRenderMarkdown_FallsBackToDefaultStyleForUnsafeOrUnknownNames(t *testing.T) {
	const text = "hello **world**"
	want := renderMarkdown(text, 80, "dark")
	for _, style := range []string{"auto", "bogus", ""} {
		if got := renderMarkdown(text, 80, style); got != want {
			t.Fatalf("renderMarkdown(%q, 80, %q) = %q, want it to fall back to the dark-style output %q", text, style, got, want)
		}
	}
}

// Regression: the live-streaming buffer must never be rendered through
// glamour until the block is complete (flushStream) — assistant text is
// flushed as Markdown, matching what the model actually sent.
func TestFlushStream_RendersMarkdown(t *testing.T) {
	m := NewModel(&fakeRunner{}, t.TempDir())
	m.handleAgentEvent(runtime.AgentEvent{Type: runtime.EventTextDelta, Text: "some **bold** text"})
	m.flushStream()

	if len(m.transcript) != 1 {
		t.Fatalf("transcript = %v, want exactly one flushed entry", m.transcript)
	}
	if !strings.Contains(m.transcript[0], "bold") {
		t.Fatalf("transcript[0] = %q, want it to contain the rendered word %q", m.transcript[0], "bold")
	}
}
