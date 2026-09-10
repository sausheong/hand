package tui

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/harness/runtime"
)

func contextInspectionLines(report runtime.ContextInspection) []string {
	lines := []string{fmt.Sprintf("Estimated context: %d tokens", report.EstimatedTokens), report.EstimateMethod}
	for _, item := range report.Contributions {
		lines = append(lines, fmt.Sprintf("%s: %d estimated tokens (%d items)", item.Name, item.EstimatedTokens, item.Count))
	}
	for _, source := range report.Sources {
		lines = append(lines, fmt.Sprintf("%s: %s (%d estimated tokens, included above)", source.Kind, source.Path, source.EstimatedTokens))
	}
	if report.OmittedSources > 0 {
		lines = append(lines, fmt.Sprintf("%d additional sources omitted from this view", report.OmittedSources))
	}
	for _, limitation := range report.Limitations {
		lines = append(lines, limitation)
	}
	return lines
}
func (m *Model) runContext(args []string) tea.Cmd {
	if len(args) != 0 || m.controller == nil {
		m.appendNotice("Usage: /context", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	controller := m.controller
	return m.startSessionOperation("context", func(ctx context.Context) sessionChangedMsg {
		report, err := controller.InspectContext(ctx)
		if err != nil {
			return sessionChangedMsg{preserveView: true, err: err}
		}
		return sessionChangedMsg{preserveView: true, lines: contextInspectionLines(report)}
	})
}
