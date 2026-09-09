package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/app"
)

// A terminal preview selects one exact path, including spaces. RPC supports
// batches. Double quotes use Go/JSON-style escapes; no shell expansion occurs.
func parseRestorePreview(text string) (string, string, error) {
	id, path, ok := strings.Cut(strings.TrimSpace(text), " ")
	path = strings.TrimSpace(path)
	if !ok || id == "" || len(id) > 128 || path == "" || len(path) > 4096 {
		return "", "", fmt.Errorf("Usage: /restore-preview RUN-ID PATH")
	}
	if strings.HasPrefix(path, "\"") {
		value, err := strconv.Unquote(path)
		if err != nil {
			return "", "", fmt.Errorf("invalid quoted path: %w", err)
		}
		path = value
	}
	if path == "" {
		return "", "", fmt.Errorf("restore path is empty")
	}
	return id, path, nil
}
func (m *Model) runRestorePreview(text string) tea.Cmd {
	id, path, err := parseRestorePreview(text)
	if err != nil || m.controller == nil {
		message := "Checkpoint controller unavailable"
		if err != nil {
			message = err.Error()
		}
		m.appendNotice(sanitizeForTerminal(message), "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	controller := m.controller
	return m.startSessionOperation("restore-preview", func(ctx context.Context) sessionChangedMsg {
		preview, err := controller.PreviewCheckpointRestore(ctx, id, []string{path})
		if err != nil {
			return sessionChangedMsg{preserveView: true, err: err}
		}
		return sessionChangedMsg{preserveView: true, lines: restorePreviewLines(preview), restoreReview: &preview}
	})
}
func restorePreviewLines(preview app.CheckpointRestorePreview) []string {
	lines := []string{"Restore preview for run " + sanitizeForTerminal(preview.RunID)}
	for _, a := range preview.Actions {
		if a.Conflict != "" {
			lines = append(lines, fmt.Sprintf("conflict %s: %s", sanitizeForTerminal(a.Path), sanitizeForTerminal(a.Conflict)))
			continue
		}
		line := fmt.Sprintf("would %s %s", a.Operation, sanitizeForTerminal(a.Path))
		if a.Restore != nil {
			line += fmt.Sprintf(" (%d bytes, mode %04o)", a.Restore.Size, a.Restore.Mode)
		}
		lines = append(lines, line)
	}
	if preview.Conflicts {
		lines = append(lines, "Preview only. Resolve conflicts before restoring.")
	} else {
		lines = append(lines, "Preview only. To apply these exact changes: /restore-confirm "+preview.Current, "To discard this preview: /restore-cancel")
	}
	return lines
}

func (m *Model) runRestoreConfirm(token string) tea.Cmd {
	review := m.restoreReview
	if review == nil || review.Conflicts || token != review.Current || token == "" || m.controller == nil {
		m.appendNotice("No matching conflict-free restore preview. Run /restore-preview first.", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	// Consume before dispatch; cancellation/errors must never leave a reusable
	// confirmation that might repeat a partially applied transaction.
	m.restoreReview = nil
	confirmed := *review
	controller := m.controller
	return m.startSessionOperation("restore", func(ctx context.Context) sessionChangedMsg {
		files, err := controller.ApplyCheckpointRestore(ctx, confirmed)
		lines := []string{}
		for _, f := range files {
			status := "not applied"
			if f.Applied {
				status = "applied"
			}
			lines = append(lines, "restore "+status+" "+sanitizeForTerminal(f.Path))
			if f.RecoveryPath != "" {
				lines = append(lines, "Retained recovery file: "+sanitizeForTerminal(f.RecoveryPath))
			}
		}
		if err == nil {
			lines = append(lines, "Selected restore completed.")
		} else {
			lines = append(lines, "Restore did not complete. Review attempted files and recovery state before retrying.")
		}
		return sessionChangedMsg{preserveView: true, lines: lines, err: err}
	})
}
