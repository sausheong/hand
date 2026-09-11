package tui

import (
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/app"
)

func TestAutomaticCompactionSkipsAreQuiet(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	defer m.CloseApplication()
	for _, reason := range []string{"too_short", "below_threshold", "cancelled", ""} {
		m.renderApplicationEvent(app.Event{Kind: "compaction_start"})
		m.renderApplicationEvent(app.Event{Kind: "compaction_skipped", Details: app.Details{Skipped: reason}})
	}
	m.renderApplicationEvent(app.Event{Kind: "compaction_done", Details: app.Details{CompactionPresent: true, Compacted: false}})
	if len(m.transcript) != 0 {
		t.Fatal(m.transcript)
	}
	m.renderApplicationEvent(app.Event{Kind: "compaction_skipped", Details: app.Details{Skipped: "summarizer_error"}})
	if !strings.Contains(strings.Join(m.transcript, "\n"), "could not finish") {
		t.Fatal("failure was hidden")
	}
}

func TestAutomaticCompactionDoesNotInventTokenSavings(t *testing.T) {
	for _, tc := range []struct {
		before, after int
		want          string
	}{{0, 0, "Older context condensed automatically"}, {5000, 1000, "Context condensed automatically: 5000 → 1000 tokens"}, {1000, 1200, "Older context condensed automatically"}} {
		b := TranscriptBlock{Kind: "compaction", State: "automatic", TokensBefore: tc.before, TokensAfter: tc.after}
		got := b.render(80, "")
		if !strings.Contains(got, tc.want) {
			t.Fatal(got)
		}
	}
	manual := TranscriptBlock{Kind: "compaction", State: "skipped", Text: "too_short"}
	if !strings.Contains(manual.render(80, ""), "No summary needed") {
		t.Fatal("manual /compact lost its response")
	}
}
