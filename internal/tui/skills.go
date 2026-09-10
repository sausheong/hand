package tui

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) runReload(args []string) tea.Cmd {
	if len(args) == 2 && m.controller != nil && m.controller.Extensions != nil {
		host := m.controller.Extensions
		return m.startSessionOperation("extension-reload", func(ctx context.Context) sessionChangedMsg {
			report, err := host.ReloadFile(ctx, args[0], args[1])
			return sessionChangedMsg{preserveView: true, err: err, lines: []string{fmt.Sprintf("Extension reload committed=%t; started=%d reused=%d removed=%d", report.Committed, len(report.Started), len(report.Reused), len(report.Removed))}}
		})
	}
	if len(args) != 0 || m.controller == nil {
		m.appendNotice("Usage: /reload (skills) | /reload REVIEW_FILE APPROVED_SHA256 (extensions)", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	controller := m.controller
	return m.startSessionOperation("reload", func(ctx context.Context) sessionChangedMsg {
		if err := controller.ReloadSkills(ctx); err != nil {
			return sessionChangedMsg{preserveView: true, err: err}
		}
		return sessionChangedMsg{preserveView: true, lines: []string{"Skill index reloaded. Updated discovery is used by the next model request."}}
	})
}
