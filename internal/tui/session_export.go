package tui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) runExportCommand(destination string) tea.Cmd {
	destination = strings.TrimSpace(destination)
	if m.controller == nil || destination == "" {
		m.appendNotice("usage: /export <new JSONL path>; paths may contain spaces", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	controller := m.controller
	return m.startSessionOperation("export", func(ctx context.Context) sessionChangedMsg {
		result := sessionChangedMsg{preserveView: true, err: controller.ExportSession(ctx, destination)}
		if result.err == nil {
			result.lines = []string{approvedStyle.Render("session exported to " + sanitizeForTerminal(destination)), toolCallStyle.Render("When moving this export, include its sibling .attachments directory if present.")}
		}
		return result
	})
}
