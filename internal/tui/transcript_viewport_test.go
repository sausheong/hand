package tui

import (
	"strings"
	"testing"
)

func TestTranscriptViewportInvalidationAndVisibleRows(t *testing.T) {
	v := transcriptViewport{Width: 12, Height: 3}
	blocks := []string{"first line\nsecond line", "a long line that wraps several times", "last"}
	v.SetBlocks(blocks, "")
	saved := &v.blocks[0].lines[0]
	v.SetBlocks(blocks, "growing")
	if saved != &v.blocks[0].lines[0] {
		t.Fatal("unchanged block was rewrapped")
	}
	v.GotoBottom()
	if !strings.Contains(v.View(), "growing") {
		t.Fatal("stream not visible at bottom")
	}
	v.GotoTop()
	if !strings.Contains(v.View(), "first") || strings.Contains(v.View(), "growing") {
		t.Fatal("visible row selection wrong")
	}
	blocks[1] = "replacement"
	v.SetBlocks(blocks, "")
	if strings.Contains(strings.Join(v.blocks[1].lines, "\n"), "wraps") {
		t.Fatal("same-length mutation retained stale layout")
	}
	v.Width = 6
	v.SetBlocks(blocks, "")
	if saved == &v.blocks[0].lines[0] {
		t.Fatal("resize did not invalidate layout")
	}
	v.SetBlocks([]string{"new session"}, "")
	if len(v.blocks) != 1 || strings.Contains(v.View(), "first") {
		t.Fatal("session replacement retained old history")
	}
	v.SetBlocks(nil, "")
	if v.totalRows() != 0 || v.YOffset != 0 {
		t.Fatal("clear retained rows")
	}
}

func TestTranscriptViewportMatchesWrappedRows(t *testing.T) {
	blocks := []string{"one\ntwo", "界界界界界界 long words", toolOKStyle.Render("styled\nresult"), "last"}
	for _, width := range []int{5, 20, 80} {
		v := transcriptViewport{Width: width, Height: 3}
		v.SetBlocks(blocks, "")
		var rows []string
		for _, b := range blocks {
			rows = append(rows, strings.Split(wrapToWidth(b, width), "\n")...)
		}
		for offset := 0; offset <= max(0, len(rows)-3); offset++ {
			v.YOffset = offset
			expected := strings.Join(rows[offset:min(offset+3, len(rows))], "\n")
			if strings.TrimSpace(v.View()) != strings.TrimSpace(expected) {
				t.Fatalf("width %d row %d: %q != %q", width, offset, v.View(), expected)
			}
		}
	}
}
