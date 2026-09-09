package tui

import (
	"context"
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/app"
)

func (m *Model) runChangesCommand(args []string) tea.Cmd {
	usage := func() tea.Cmd {
		m.appendNotice("Usage: /changes [run-ID] [offset]; requires --checkpoint-dir", "toolCallStyle")
		m.refreshViewport()
		return nil
	}
	if m.controller == nil || len(args) > 2 {
		return usage()
	}
	id := ""
	offset := 0
	if len(args) > 0 {
		id = args[0]
		if len(id) > 128 {
			return usage()
		}
	}
	if len(args) == 2 {
		var err error
		offset, err = strconv.Atoi(args[1])
		if err != nil || offset < 0 {
			return usage()
		}
	}
	controller := m.controller
	return m.startSessionOperation("changes", func(ctx context.Context) sessionChangedMsg {
		page, err := controller.CheckpointChanges(ctx, id, offset)
		if err != nil {
			return sessionChangedMsg{preserveView: true, err: err}
		}
		return sessionChangedMsg{preserveView: true, lines: checkpointChangeLines(page)}
	})
}
func checkpointChangeLines(page app.CheckpointChangesPage) []string {
	lines := []string{fmt.Sprintf("Checkpoint run %s: %d changed files", sanitizeForTerminal(page.RunID), page.Total)}
	for _, change := range page.Changes {
		path := change.Path
		if len(path) > 4096 {
			path = path[:4096] + "…"
		}
		path = sanitizeForTerminal(path)
		switch {
		case change.Before == nil && change.After != nil:
			lines = append(lines, fmt.Sprintf("added %s (%d bytes, mode %04o)", path, change.After.Size, change.After.Mode))
		case change.After == nil && change.Before != nil:
			lines = append(lines, fmt.Sprintf("removed %s (%d bytes, mode %04o)", path, change.Before.Size, change.Before.Mode))
		case change.Before != nil && change.After != nil:
			lines = append(lines, fmt.Sprintf("changed %s (%d → %d bytes, mode %04o → %04o)", path, change.Before.Size, change.After.Size, change.Before.Mode, change.After.Mode))
		}
	}
	if page.Total == 0 {
		lines = append(lines, "No file changes in the captured scope")
	}
	if page.BeforeOmissions > 0 || page.AfterOmissions > 0 {
		lines = append(lines, fmt.Sprintf("Omitted paths: %d before, %d after; excluded content is outside this comparison", page.BeforeOmissions, page.AfterOmissions))
	}
	if page.Next < page.Total {
		lines = append(lines, fmt.Sprintf("Next page: /changes %s %d", sanitizeForTerminal(page.RunID), page.Next))
	}
	lines = append(lines, "Recorded before/after changes; later edits are not shown. No files were restored.")
	return lines
}
