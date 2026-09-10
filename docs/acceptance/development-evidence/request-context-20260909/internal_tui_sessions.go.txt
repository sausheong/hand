package tui

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/llm"
	"strings"
	"time"
)

func (m *Model) runResumeCommand(args []string) tea.Cmd {
	if m.controller == nil {
		m.appendNotice("workspace sessions unavailable", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	if len(args) == 0 {
		controller := m.controller
		return m.startSessionOperation("list", func(ctx context.Context) sessionChangedMsg {
			result := sessionChangedMsg{preserveView: true}
			records, err := controller.ListSessionsContext(ctx)
			if err != nil {
				result.err = err
				return result
			}
			if len(records) == 0 {
				result.lines = append(result.lines, toolCallStyle.Render("no saved sessions"))
				return result
			}
			active := controller.SessionID()
			for _, record := range records {
				if err := ctx.Err(); err != nil {
					result.err = err
					return result
				}
				marker := " "
				if record.ID == active {
					marker = "*"
				}
				name := record.Name
				if name == "" {
					name = "(unnamed)"
				}
				result.lines = append(result.lines, toolCallStyle.Render(sanitizeForTerminal(fmt.Sprintf("%s %s  %s", marker, record.ID, name))))
			}
			return result
		})
	}
	if len(args) != 1 {
		m.appendNotice("usage: /resume [session ID]", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	return m.startSessionChange("resume", func(ctx context.Context) error { return m.controller.ResumeSessionContext(ctx, args[0]) })
}

func (m *Model) runNameCommand(name string) tea.Cmd {
	name = strings.TrimSpace(name)
	if m.controller == nil {
		m.appendNotice("workspace sessions unavailable", "errorLineStyle")
		return nil
	}
	if name == "" {
		m.appendNotice("usage: /name <session name>", "errorLineStyle")
		return nil
	}
	controller := m.controller
	return m.startSessionOperation("name", func(ctx context.Context) sessionChangedMsg {
		result := sessionChangedMsg{preserveView: true, err: controller.RenameSessionContext(ctx, name)}
		if result.err == nil {
			result.lines = []string{approvedStyle.Render("session named " + sanitizeForTerminal(name))}
		}
		return result
	})
}

// Reset transient figures rather than attributing another session's usage to
// the selected history. Persisted accounting is loaded separately when present.
func (m *Model) resetSessionView() {
	m.streamBuf.Reset()
	m.dismissApproval()
	m.lastUsage = nil
	m.lastRequestUsage = nil
	m.sessionUsage = llm.Usage{}
	m.usageRequests, m.usageUnknown = 0, 0
	m.usagePriorUnknown = false
	m.lastModelInfo = app.ModelInfo{}
	m.toolCallsThisTurn = 0
	m.lastTurnDuration = 0
	m.turnStart = time.Time{}
}
