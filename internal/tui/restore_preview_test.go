package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/checkpoints"
)

func TestRestorePreviewExactPathParsing(t *testing.T) {
	for _, text := range []string{`run path with spaces.go`, `run "path with spaces.go"`} {
		id, path, err := parseRestorePreview(text)
		if err != nil || id != "run" || path != "path with spaces.go" {
			t.Fatal(id, path, err)
		}
	}
	_, path, err := parseRestorePreview("run $(echo unsafe).go")
	if err != nil || path != "$(echo unsafe).go" {
		t.Fatal("expanded literal path", path, err)
	}
	for _, text := range []string{"", "run", `run "broken`, `run ""`} {
		if _, _, err := parseRestorePreview(text); err == nil {
			t.Fatal("invalid input accepted", text)
		}
	}
}
func TestRestorePreviewConflictPresentation(t *testing.T) {
	p := app.CheckpointRestorePreview{RunID: "run", Actions: []checkpoints.RestoreAction{{Path: "file", Operation: "replace", Restore: &checkpoints.Record{Size: 12, Mode: 0755}}, {Path: "edited", Conflict: "current file changed"}}}
	text := strings.Join(restorePreviewLines(p), "\n")
	for _, want := range []string{"would replace file (12 bytes, mode 0755)", "conflict edited", "Preview only"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
}
func TestRestorePreviewOwnedWorker(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	old := m.identity
	cmd := m.runRestorePreview("run file")
	if cmd == nil || !m.sessionChanging {
		t.Fatal("missing worker")
	}
	m.Update(cmd())
	if m.sessionChanging || m.identity != old {
		t.Fatal("preview altered session or leaked worker")
	}
}

func TestRestoreConfirmationConsumedAndCancellation(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	preview := &app.CheckpointRestorePreview{RunID: "run", Current: "reviewed-digest"}
	m.restoreReview = preview
	if cmd := m.runRestoreConfirm("wrong"); cmd != nil || m.restoreReview != preview {
		t.Fatal("mismatched token dispatched")
	}
	m.running = true
	if cmd := m.handleCommand("/restore-confirm reviewed-digest"); cmd != nil || m.restoreReview != preview {
		t.Fatal("restore dispatched during run")
	}
	m.running = false
	cmd := m.runRestoreConfirm("reviewed-digest")
	if cmd == nil || m.restoreReview != nil || !m.sessionChanging {
		t.Fatal("confirmation not consumed before worker")
	}
	m.Update(cmd())
	if m.sessionChanging || m.restoreReview != nil {
		t.Fatal("failed restore left live confirmation")
	}
	if cmd := m.runRestoreConfirm("reviewed-digest"); cmd != nil {
		t.Fatal("confirmation replay dispatched")
	}
	m.restoreReview = preview
	m.handleCommand("/restore-cancel")
	if m.restoreReview != nil {
		t.Fatal("cancel retained preview")
	}
}

func TestRestoreFailureDisplaysPartialResults(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	cmd := m.startSessionOperation("restore", func(context.Context) sessionChangedMsg {
		return sessionChangedMsg{preserveView: true, lines: []string{"restore applied first.txt", "Retained recovery file: backup"}, err: errors.New("second file conflict")}
	})
	m.Update(cmd())
	text := strings.Join(m.transcript, "\n")
	if !strings.Contains(text, "restore applied first.txt") || !strings.Contains(text, "second file conflict") {
		t.Fatal(text)
	}
}
