package tui

import (
	"context"
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/app"
)

func (m *Model) runRecoveries(args []string) tea.Cmd {
	offset := 0
	if len(args) == 1 {
		var err error
		offset, err = strconv.Atoi(args[0])
		if err != nil {
			offset = -1
		}
	}
	if len(args) > 1 || offset < 0 || m.controller == nil {
		m.appendNotice("Usage: /recoveries [offset]; requires --checkpoint-dir", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	controller := m.controller
	return m.startSessionOperation("recoveries", func(ctx context.Context) sessionChangedMsg {
		page, err := controller.CheckpointRecoveries(ctx, offset)
		if err != nil {
			return sessionChangedMsg{preserveView: true, err: err}
		}
		return sessionChangedMsg{preserveView: true, recoveryPage: &page, lines: recoveryLines(page)}
	})
}
func recoveryLines(page app.CheckpointRecoveryPage) []string {
	lines := []string{fmt.Sprintf("Restore recovery: %d records", page.Total)}
	for _, r := range page.Recoveries {
		id := sanitizeForTerminal(r.Event.RecoveryName)
		lines = append(lines, fmt.Sprintf("%s %s: %s", id, sanitizeForTerminal(r.Event.Path), sanitizeForTerminal(r.State)))
		if r.Reason != "" {
			lines = append(lines, sanitizeForTerminal(r.Reason))
		}
		action := ""
		if r.State == "prepared" {
			action = "cancel"
		}
		if r.State == "applied_unrecorded" {
			action = "acknowledge"
		}
		if action != "" {
			lines = append(lines, "To confirm this resolution: /recovery-resolve "+action+" "+id)
		}
	}
	if page.Next < page.Total {
		lines = append(lines, fmt.Sprintf("Next page: /recoveries %d", page.Next))
	}
	lines = append(lines, "Resolution preserves target and recovery files. Conflicts require inspection; no automatic retry.")
	return lines
}
func (m *Model) runRecoveryResolve(args []string) tea.Cmd {
	if len(args) != 2 || (args[0] != "acknowledge" && args[0] != "cancel") || m.recoveryPage == nil || m.controller == nil {
		m.appendNotice("Inspect /recoveries first, then use /recovery-resolve acknowledge|cancel RECOVERY-ID", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	for _, r := range m.recoveryPage.Recoveries {
		if r.Event.RecoveryName != args[1] {
			continue
		}
		if (args[0] == "cancel" && r.State != "prepared") || (args[0] == "acknowledge" && r.State != "applied_unrecorded") {
			break
		}
		controller := m.controller
		action := args[0]
		reviewed := r
		m.recoveryPage = nil
		return m.startSessionOperation("recovery-resolve", func(ctx context.Context) sessionChangedMsg {
			err := controller.ResolveCheckpointRecovery(ctx, action, reviewed)
			if err != nil {
				return sessionChangedMsg{preserveView: true, err: err}
			}
			return sessionChangedMsg{preserveView: true, lines: []string{"Recovery resolution recorded; files retained. Run /recoveries to inspect."}}
		})
	}
	m.appendNotice("No matching unresolved record on the displayed recovery page.", "errorLineStyle")
	m.refreshViewport()
	return nil
}
