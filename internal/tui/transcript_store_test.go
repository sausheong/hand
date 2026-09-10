package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

func TestTranscriptProjectionHasSingleWriter(t *testing.T) {
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		name := file.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "transcript_blocks.go" {
			continue
		}
		tree, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(tree, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, lhs := range assignment.Lhs {
				if index, ok := lhs.(*ast.IndexExpr); ok {
					lhs = index.X
				}
				selector, ok := lhs.(*ast.SelectorExpr)
				if ok && selector.Sel.Name == "transcript" {
					t.Errorf("%s writes transcript projection outside source store", name)
				}
			}
			return true
		})
	}
}

func TestTranscriptSourceOwnsProjectionAndClear(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.appendNotice("administrative notice", "toolCallStyle")
	m.appendSourceBlock(TranscriptBlock{Kind: "user", Text: "original"})
	m.transcript[1] = "corrupted cached presentation"
	m.refreshViewport()
	if m.sourceBlocks[1].Block.Text != "original" || !strings.Contains(m.transcript[1], "original") {
		t.Fatal("cache became source of truth")
	}
	m.clearTranscript()
	m.refreshViewport()
	if len(m.transcript) != 0 || len(m.sourceBlocks) != 0 || m.viewport.totalRows() != 0 {
		t.Fatal("clear left stale source or presentation")
	}
}
