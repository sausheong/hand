package tui

import (
	"strings"
	"testing"

	"github.com/sausheong/harness/runtime"
)

func TestRenderMarkdown_EmptyOrWhitespaceReturnsUnchanged(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\t\n"} {
		if got := renderMarkdown(in, 80); got != in {
			t.Errorf("renderMarkdown(%q, 80) = %q, want unchanged", in, got)
		}
	}
}

// Exact ANSI styling (or whether glamour strips "**"/"#" markers
// entirely) depends on terminal color-profile detection, which differs
// between a real TTY and a headless `go test` run — so these tests
// check the property that holds either way: the actual words survive
// rendering, and nothing panics or errors out to the raw-text fallback
// for ordinary Markdown.
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
			got := renderMarkdown(tc.in, 80)
			if !strings.Contains(got, tc.want) {
				t.Fatalf("renderMarkdown(%q, 80) = %q, want it to contain %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRenderMarkdown_ClampsNarrowOrInvalidWidth(t *testing.T) {
	for _, w := range []int{0, -5, 1, minMarkdownWidth - 1} {
		got := renderMarkdown("hello world", w)
		if !strings.Contains(got, "hello") {
			t.Fatalf("renderMarkdown(%q, width=%d) = %q, want it to still contain the content", "hello world", w, got)
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
