package tui

import (
	"github.com/charmbracelet/lipgloss"
	"sort"
	"strings"
)

// transcriptViewport indexes cached block rows. Only changed blocks are wrapped;
// View visits the visible row interval rather than scanning the entire history.
// A width change invalidates layout without discarding the source blocks.
type transcriptViewport struct {
	Width, Height, YOffset int
	width                  int
	blocks                 []transcriptLayout
	ends                   []int
}
type transcriptLayout struct {
	source string
	lines  []string
}

func (v *transcriptViewport) SetBlocks(blocks []string, stream string) {
	n := len(blocks)
	emptyPrefix := n == 0 && stream != ""
	if emptyPrefix {
		n++
	}
	if stream != "" {
		n++
	}
	widthChanged := v.width != v.Width
	v.width = v.Width
	if n < len(v.blocks) {
		clear(v.blocks[n:])
		v.blocks = v.blocks[:n]
		v.ends = v.ends[:n]
	}
	for len(v.blocks) < n {
		v.blocks = append(v.blocks, transcriptLayout{})
		v.ends = append(v.ends, 0)
	}
	rows := 0
	for i := 0; i < n; i++ {
		source := ""
		if i < len(blocks) {
			source = blocks[i]
		} else if !emptyPrefix || i > 0 {
			source = stream
		}
		cached := &v.blocks[i]
		if widthChanged || cached.lines == nil || cached.source != source {
			cached.source = source
			cached.lines = strings.Split(wrapToWidth(source, v.Width), "\n")
		}
		rows += len(cached.lines)
		v.ends[i] = rows
	}
	v.YOffset = min(max(0, v.YOffset), v.maxOffset())
}
func (v *transcriptViewport) totalRows() int {
	if len(v.ends) == 0 {
		return 0
	}
	return v.ends[len(v.ends)-1]
}
func (v *transcriptViewport) maxOffset() int { return max(0, v.totalRows()-max(1, v.Height)) }
func (v *transcriptViewport) AtBottom() bool { return v.YOffset >= v.maxOffset() }
func (v *transcriptViewport) GotoBottom()    { v.YOffset = v.maxOffset() }
func (v *transcriptViewport) GotoTop()       { v.YOffset = 0 }
func (v *transcriptViewport) HalfPageUp()    { v.YOffset = max(0, v.YOffset-max(1, v.Height/2)) }
func (v *transcriptViewport) HalfPageDown() {
	v.YOffset = min(v.maxOffset(), v.YOffset+max(1, v.Height/2))
}
func (v *transcriptViewport) View() string {
	height := max(1, v.Height)
	start := min(max(0, v.YOffset), v.maxOffset())
	index := sort.Search(len(v.ends), func(i int) bool { return v.ends[i] > start })
	lines := make([]string, 0, height)
	for index < len(v.blocks) && len(lines) < height {
		base := 0
		if index > 0 {
			base = v.ends[index-1]
		}
		offset := max(0, start-base)
		block := v.blocks[index].lines
		end := min(len(block), offset+height-len(lines))
		lines = append(lines, block[offset:end]...)
		index++
	}
	return lipgloss.NewStyle().Width(max(1, v.Width)).Height(height).Render(strings.Join(lines, "\n"))
}
