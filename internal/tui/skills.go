package tui

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
)

// Keep the model's discovery index unchanged; only its terminal presentation
// gets hierarchy and spacing. Source paths and descriptions remain complete.
func renderSkillsIndex(index string) string {
	var entries []string
	for _, line := range strings.Split(index, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "## Skills" {
			entries = append(entries, userLineStyle.Render("Skills")+"\n"+toolCallStyle.Render("Scroll with the wheel, Page Up/Down or Ctrl+U/D."))
			continue
		}
		if strings.HasPrefix(line, "- ") {
			name, description, ok := strings.Cut(strings.TrimPrefix(line, "- "), ": ")
			if ok {
				source := ""
				if at := strings.LastIndex(description, " (source: "); at >= 0 && strings.HasSuffix(description, ")") {
					source = description[at+len(" (source: ") : len(description)-1]
					description = description[:at]
				}
				entry := userLineStyle.Render(name) + "\n" + description
				if source != "" {
					entry += "\n" + toolCallStyle.Render("Source: "+source)
				}
				entries = append(entries, entry)
				continue
			}
		}
		entries = append(entries, toolCallStyle.Render(line))
	}
	return strings.Join(entries, "\n\n")
}

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
