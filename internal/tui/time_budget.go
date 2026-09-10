package tui

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/app"
	"strconv"
	"time"
)

func timeBudgetLines(view app.TimeBudgetView) []string {
	if !view.Configured {
		return []string{"Session time budget: not configured", view.Disclosure, "Set an allowance: /time-budget SECONDS (1-2592000)."}
	}
	status := "active"
	if view.Expired {
		status = "expired; an explicit new allowance is required"
	}
	return []string{"Session time budget: " + status, "Deadline: " + view.Deadline.UTC().Format(time.RFC3339Nano), fmt.Sprintf("Remaining: %d milliseconds", view.RemainingMillis), view.Disclosure, "Renew explicitly: /time-budget SECONDS. This starts a new wall-clock allowance; prior token and monetary charges remain."}
}
func (m *Model) runTimeBudget(args []string) tea.Cmd {
	valid := len(args) == 0
	var seconds int64
	if len(args) == 1 {
		var err error
		seconds, err = strconv.ParseInt(args[0], 10, 64)
		valid = err == nil && seconds > 0 && seconds <= 2592000
	}
	if !valid || m.controller == nil {
		m.appendNotice("Usage: /time-budget [SECONDS] (1-2592000; idle time counts)", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	controller := m.controller
	return m.startSessionOperation("time-budget", func(ctx context.Context) sessionChangedMsg {
		if len(args) == 1 {
			if err := controller.DecideTimeBudget(ctx, seconds); err != nil {
				return sessionChangedMsg{preserveView: true, err: err}
			}
			return sessionChangedMsg{preserveView: true, lines: []string{fmt.Sprintf("New session time allowance: %d seconds, including idle time. Prior charges remain. Use /time-budget to inspect the deadline.", seconds)}}
		}
		view, err := controller.TimeBudget(ctx)
		return sessionChangedMsg{preserveView: true, err: err, lines: timeBudgetLines(view)}
	})
}
