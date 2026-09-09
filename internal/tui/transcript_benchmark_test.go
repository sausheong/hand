package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"sort"
	"testing"
	"time"
)

// Each measured event includes delivery of a text delta ahead of a key, so a
// fast textarea cannot conceal expensive transcript work on the same UI loop.
func BenchmarkTranscript10000Blocks(b *testing.B) {
	m := NewModel(nil, b.TempDir())
	b.Cleanup(m.CloseApplication)
	m.resize(80, 24)
	for i := 0; i < 10000; i++ {
		m.appendSourceBlock(TranscriptBlock{Kind: "notice", Text: fmt.Sprintf("Block %d: source inspection and tool output with enough text to wrap at terminal width.\nSecond line.", i)})
	}
	if len(m.transcript) != 10000 || len(m.sourceBlocks) != 10000 {
		b.Fatal("benchmark did not retain 10000 source blocks")
	}
	m.refreshViewport()
	samples := make([]float64, 0, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Keep typing below the editor limit so each measured event changes
		// input, instead of mostly measuring rejected keystrokes.
		if len(m.textarea.Value()) >= 128 {
			m.textarea.Reset()
		}
		before := len(m.textarea.Value())
		start := time.Now()
		m.streamBuf.WriteString("delta ")
		m.refreshViewport()
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
		_ = m.View()
		samples = append(samples, float64(time.Since(start).Nanoseconds())/1e6)
		if len(m.textarea.Value()) != before+1 {
			b.Fatal("measured key did not change input")
		}
	}
	b.StopTimer()
	sort.Float64s(samples)
	if len(samples) > 0 {
		b.ReportMetric(samples[(95*len(samples)+99)/100-1], "p95-ms")
	}
	b.Logf("latency_ms_sorted=%v", samples)
}
