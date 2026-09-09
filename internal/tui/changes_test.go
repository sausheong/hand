package tui

import (
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/checkpoints"
)

func TestCheckpointChangesFormatting(t *testing.T) {
	p := app.CheckpointChangesPage{RunID: "run", Total: 40, Next: 32, BeforeOmissions: 1, AfterOmissions: 2, Changes: []checkpoints.Change{{Path: "add", After: &checkpoints.Record{Size: 7, Mode: 0755}}, {Path: "remove", Before: &checkpoints.Record{Size: 4, Mode: 0600}}, {Path: "mode", Before: &checkpoints.Record{Size: 2, Mode: 0644}, After: &checkpoints.Record{Size: 2, Mode: 0755}}}}
	text := strings.Join(checkpointChangeLines(p), "\n")
	for _, want := range []string{"added add", "removed remove", "0644 → 0755", "/changes run 32", "Omitted paths: 1 before, 2 after", "later edits are not shown"} {
		if !strings.Contains(text, want) {
			t.Fatal("missing", want, text)
		}
	}
}
func TestChangesCommandUsesJoinedSessionWorker(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	identity := m.identity
	cmd := m.runChangesCommand(nil)
	if cmd == nil || !m.sessionChanging {
		t.Fatal("changes not running as owned operation")
	}
	m.Update(cmd())
	if m.sessionChanging || m.identity != identity {
		t.Fatal("inspection changed session identity or retained worker")
	}
	if !strings.Contains(strings.Join(m.transcript, "\n"), "checkpoint capture is not configured") {
		t.Fatal("missing unavailable checkpoint message")
	}
}
