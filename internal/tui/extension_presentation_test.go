package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/sausheong/hand/extension/protocol"
)

func TestExtensionPresentationLiteralContentAndResize(t *testing.T) {
	literal := "[a link](https://example.invalid) **literal**"
	p := protocol.Presentation{Blocks: []protocol.Block{
		{Kind: "text", Text: literal},
		{Kind: "code", Language: "go", Text: "if ready {\n    run()\n}\n```"},
		{Kind: "list", Items: []string{"one two three four five six seven eight nine ten", "second"}},
	}}
	blocks, err := extensionPresentationBlocks("fixture", p)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 5 || blocks[0].Text != "Extension fixture" {
		t.Fatalf("blocks: %+v", blocks)
	}
	if got := blocks[1].render(100, "dark"); !strings.Contains(got, literal) {
		t.Fatalf("text interpreted: %q", got)
	}
	code := blocks[2].render(50, "dark")
	for _, want := range []string{"code (go)", "    run()", "```", "┌", "└"} {
		if !strings.Contains(code, want) {
			t.Fatalf("code lost %q: %q", want, code)
		}
	}
	for _, width := range []int{1, 2, 5, 6, 12, 40} {
		for _, block := range blocks[1:] {
			rendered := block.render(width, "dark")
			for _, line := range strings.Split(rendered, "\n") {
				if lipgloss.Width(line) > width {
					t.Fatalf("width %d: %q", width, line)
				}
			}
		}
	}
	m := Model{termWidth: 60, markdownStyle: "dark"}
	for _, block := range blocks {
		m.appendSourceBlock(block)
	}
	before := m.transcript[3]
	m.termWidth = 12
	m.refreshSourceBlocks()
	if m.transcript[3] == before || m.sourceBlocks[3].Block.Text != p.Blocks[2].Items[0] {
		t.Fatal("resize failed or changed source")
	}
	if !strings.Contains(m.transcript[3], "\n  ") {
		t.Fatal("list lost hanging indent")
	}
	// No caller-owned list slices are retained in transcript source.
	p.Blocks[2].Items[0] = "mutated"
	if blocks[3].Text == "mutated" {
		t.Fatal("source aliased caller")
	}
}

func TestExtensionPresentationRejectsTerminalAuthority(t *testing.T) {
	for _, block := range []protocol.Block{
		{Kind: "text", Text: "\x1b]52;c;YQ==\a"},
		{Kind: "code", Text: "\x1b[2J"},
		{Kind: "list", Items: []string{"spoof\u202e"}},
		{Kind: "approval", Text: "approved"},
		{Kind: "code", Text: "ok", Language: "go\x1b[2J"},
	} {
		if blocks, err := extensionPresentationBlocks("fixture", protocol.Presentation{Blocks: []protocol.Block{block}}); err == nil || blocks != nil {
			t.Fatalf("unsafe presentation accepted: %+v", block)
		}
	}
}
