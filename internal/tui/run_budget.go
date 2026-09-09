package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/app"
)

const runBudgetUsage = "Usage: /run-budget [ID] | /run-budget select ID | /run-budget tokens ID TOTAL | /run-budget cost ID CURRENCY TOTAL strict|advisory | /run-budget time ID SECONDS"

func runBudgetLines(v app.RunBudgetView) []string {
	if v.ID == "" {
		return []string{"No run budget selected.", v.Disclosure, runBudgetUsage}
	}
	status := "not selected"
	if v.Selected {
		status = "selected"
	}
	lines := []string{fmt.Sprintf("Run budget %s: %s; configured: %t", sanitizeForTerminal(v.ID), status, v.Configured),
		fmt.Sprintf("Tokens: absolute ceiling %d; committed %d; exhausted %t; attempts %d; uncertain %d", v.Tokens.Limit, v.Tokens.Committed, v.Tokens.Exhausted, v.Tokens.Attempts, v.Tokens.Uncertain),
		fmt.Sprintf("Cost: %s ceiling %s; committed %s; strict %t; exhausted %t", sanitizeForTerminal(v.Cost.Currency), costAmount(v.Cost.LimitNano), costAmount(v.Cost.CommittedNano), v.Cost.Strict, v.Cost.Exhausted),
		fmt.Sprintf("Compaction cost %s; unpriced %d; uncertain/outstanding %d", costAmount(v.Cost.CompactionNano), v.Cost.Unpriced, v.Cost.Uncertain), v.Cost.Disclosure}
	if v.Time.Configured {
		lines = append(lines, fmt.Sprintf("Deadline: %s; remaining %d milliseconds; expired: %t", v.Time.Deadline.UTC().Format(time.RFC3339Nano), v.Time.RemainingMillis, v.Time.Expired), v.Time.Disclosure)
	} else {
		lines = append(lines, "Run time budget: not configured")
	}
	return append(lines, v.Disclosure, "Exhaustion requires an explicit decision. Selecting another configured ID changes the logical run; prior charges remain.", runBudgetUsage)
}
func (m *Model) runRunBudget(args []string) tea.Cmd {
	action, id := "inspect", ""
	valid := true
	var amount int64
	var currency string
	var strict bool
	switch len(args) {
	case 0:
	case 1:
		id = args[0]
	case 2:
		action, id = args[0], args[1]
		valid = action == "select"
	case 3:
		action, id = args[0], args[1]
		var err error
		amount, err = strconv.ParseInt(args[2], 10, 64)
		valid = err == nil && amount > 0 && ((action == "tokens" && amount <= 1<<50) || (action == "time" && amount <= 2592000))
	case 5:
		action, id, currency = args[0], args[1], args[2]
		var err error
		amount, err = parseCostAmount(args[3])
		strict = args[4] == "strict"
		valid = action == "cost" && err == nil && len(currency) == 3 && strings.IndexFunc(currency, func(r rune) bool { return r < 'A' || r > 'Z' }) < 0 && (strict || args[4] == "advisory")
	default:
		valid = false
	}
	if id != "" {
		valid = valid && len(id) <= 64 && strings.IndexFunc(id, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
		}) < 0
	}
	if !valid || m.controller == nil {
		m.appendNotice(runBudgetUsage, "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	c := m.controller
	return m.startSessionOperation("run-budget", func(ctx context.Context) sessionChangedMsg {
		var err error
		notice := ""
		switch action {
		case "inspect":
			v, e := c.InspectRunBudget(ctx, id)
			return sessionChangedMsg{preserveView: true, err: e, lines: runBudgetLines(v)}
		case "select":
			err = c.SelectRunBudget(ctx, id)
			notice = "Selected run budget " + id + ". Selection persists across prompts and restart; existing charges remain."
		case "tokens":
			err = c.DecideRunTokenBudget(ctx, id, amount)
			notice = fmt.Sprintf("Run %s absolute token ceiling: %d. Prior charges remain; selection is unchanged.", id, amount)
		case "cost":
			err = c.DecideRunCostBudget(ctx, id, currency, amount, strict)
			notice = fmt.Sprintf("Run %s absolute cost ceiling: %s %s; strict: %t. Prior charges remain; selection is unchanged.", id, currency, costAmount(amount), strict)
		case "time":
			err = c.DecideRunTimeBudget(ctx, id, amount)
			notice = fmt.Sprintf("Run %s new wall-clock allowance: %d seconds, including idle time. Selection is unchanged.", id, amount)
		}
		return sessionChangedMsg{preserveView: true, err: err, lines: []string{notice}}
	})
}
