package tui

import (
	"context"
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/app"
)

func tokenBudgetLines(view app.TokenBudgetView) []string {
	mode := "active"
	if view.Limit == 0 {
		mode = "not configured"
	} else if view.Exhausted {
		mode = "exhausted; an explicit new ceiling is required"
	}
	return []string{
		fmt.Sprintf("Session token budget: %s", mode),
		fmt.Sprintf("Absolute ceiling: %d; committed including reservations: %d", view.Limit, view.Committed),
		fmt.Sprintf("Provider attempts: %d; uncertain or outstanding: %d", view.Attempts, view.Uncertain),
		view.EstimateMethod,
		"Set or resume with /budget-tokens TOTAL. TOTAL includes all prior charges and reservations; it is not an additional allowance.",
	}
}
func (m *Model) runTokenBudget(name string, args []string) tea.Cmd {
	valid := name == "/budget" && len(args) == 0
	var limit int64
	if name == "/budget-tokens" && len(args) == 1 {
		var err error
		limit, err = strconv.ParseInt(args[0], 10, 64)
		valid = err == nil && limit > 0 && limit <= 1<<50
	}
	if !valid || m.controller == nil {
		m.appendNotice("Usage: /budget | /budget-tokens TOTAL (positive absolute session token ceiling)", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	controller := m.controller
	return m.startSessionOperation("token-budget", func(ctx context.Context) sessionChangedMsg {
		if name == "/budget-tokens" {
			if err := controller.DecideTokenBudget(ctx, limit); err != nil {
				return sessionChangedMsg{preserveView: true, err: err}
			}
			return sessionChangedMsg{preserveView: true, lines: []string{fmt.Sprintf("Session token ceiling set to %d total tokens. Existing charges and uncertain reservations remain. Use /budget to inspect.", limit)}}
		}
		view, err := controller.TokenBudget(ctx)
		if err != nil {
			return sessionChangedMsg{preserveView: true, err: err}
		}
		return sessionChangedMsg{preserveView: true, lines: tokenBudgetLines(view)}
	})
}
