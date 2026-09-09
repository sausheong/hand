package tui

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/harness/runtime"
	"strings"
)

const contextStateUsage = "Usage: /state | /state set ID objective|decision|unresolved_work TEXT | /state evidence ID REFERENCE TEXT | /state remove ID"

func (m *Model) runContextState(args []string) tea.Cmd {
	action := "inspect"
	valid := true
	var item runtime.ContextStateItem
	if len(args) > 0 {
		action = args[0]
		switch {
		case action == "set" && len(args) >= 4:
			item = runtime.ContextStateItem{ID: args[1], Kind: args[2], Text: strings.Join(args[3:], " ")}
			valid = item.Kind == "objective" || item.Kind == "decision" || item.Kind == "unresolved_work"
		case action == "evidence" && len(args) >= 4:
			item = runtime.ContextStateItem{ID: args[1], Kind: "verification_reference", Reference: args[2], Text: strings.Join(args[3:], " ")}
		case action == "remove" && len(args) == 2:
			item.ID = args[1]
		default:
			valid = false
		}
	}
	if !valid || m.controller == nil {
		m.appendNotice(contextStateUsage, "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	controller := m.controller
	return m.startSessionOperation("task-state", func(ctx context.Context) sessionChangedMsg {
		if action != "inspect" {
			err := controller.UpdateContextItem(ctx, item, action == "remove")
			return sessionChangedMsg{preserveView: true, err: err, lines: []string{"Structured task state updated. Use /state to inspect the current set."}}
		}
		state, err := controller.ContextState(ctx)
		if err != nil {
			return sessionChangedMsg{preserveView: true, err: err}
		}
		lines := []string{fmt.Sprintf("Structured task state: %d items, revision %d (session-wide; survives compaction)", len(state.Items), state.Revision)}
		for _, item := range state.Items {
			lines = append(lines, fmt.Sprintf("%s [%s]: %s", sanitizeForTerminal(item.Kind), sanitizeForTerminal(item.ID), sanitizeForTerminal(item.Text)))
			if item.Reference != "" {
				lines = append(lines, "Evidence locator: "+sanitizeForTerminal(item.Reference))
			}
		}
		lines = append(lines, "Evidence references do not establish current verification; check their scope and workspace version.", contextStateUsage)
		return sessionChangedMsg{preserveView: true, lines: lines}
	})
}
