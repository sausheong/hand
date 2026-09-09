package tui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/budget"
)

func parseCostAmount(value string) (int64, error) {
	parts := strings.Split(value, ".")
	if len(parts) > 2 || len(parts[0]) == 0 || len(parts[0]) > 7 {
		return 0, errors.New("invalid decimal amount")
	}
	for _, part := range parts {
		if part == "" || strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return 0, errors.New("amount requires decimal digits")
		}
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if len(fraction) > 9 {
			return 0, errors.New("amount supports at most nine decimal places")
		}
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	nanoFraction, err := strconv.ParseInt(fraction+strings.Repeat("0", 9-len(fraction)), 10, 64)
	if err != nil {
		return 0, err
	}
	nano := whole*1000000000 + nanoFraction
	if nano <= 0 || nano > 1<<50 {
		return 0, errors.New("amount outside supported positive ceiling")
	}
	return nano, nil
}
func costAmount(nano int64) string {
	whole := strconv.FormatInt(nano/1000000000, 10)
	if nano%1000000000 == 0 {
		return whole
	}
	return whole + "." + strings.TrimRight(fmt.Sprintf("%09d", nano%1000000000), "0")
}
func costBudgetLines(view app.CostBudgetView) []string {
	mode := "advisory"
	if view.Strict {
		mode = "strict"
	}
	if view.LimitNano == 0 {
		mode = "not configured"
	}
	if view.Exhausted {
		mode += "; exhausted"
	}
	currency := sanitizeForTerminal(view.Currency)
	return []string{fmt.Sprintf("Session monetary budget: %s", mode),
		fmt.Sprintf("%s absolute ceiling: %s; committed including reservations: %s", currency, costAmount(view.LimitNano), costAmount(view.CommittedNano)),
		fmt.Sprintf("Compaction: %s %s; attempts: %d; unpriced: %d; uncertain/outstanding: %d", currency, costAmount(view.CompactionNano), view.Attempts, view.Unpriced, view.Uncertain),
		view.Disclosure, "Set or resume: /cost CURRENCY TOTAL strict|advisory. TOTAL includes all existing charges; it is not an additional allowance."}
}
func priceLines(prices []budget.PriceSnapshot) []string {
	lines := []string{fmt.Sprintf("Installed route tariffs: %d", len(prices))}
	for _, p := range prices {
		lines = append(lines, fmt.Sprintf("%s/%s at %s", sanitizeForTerminal(p.Provider), sanitizeForTerminal(p.Model), sanitizeForTerminal(p.Destination)),
			fmt.Sprintf("%s per million tokens: input %s; output %s; cache write %s; cache read %s; fixed per attempt %s", sanitizeForTerminal(p.Currency), costAmount(p.InputNanoPerMillion), costAmount(p.OutputNanoPerMillion), costAmount(p.CacheWriteNanoPerMillion), costAmount(p.CacheReadNanoPerMillion), costAmount(p.FixedNano)),
			fmt.Sprintf("Source: %s; version: %s; valid %s to %s; all charges bounded: %t", sanitizeForTerminal(p.Source), sanitizeForTerminal(p.Version), p.EffectiveAt.Format(time.RFC3339), p.ExpiresAt.Format(time.RFC3339), p.AllChargesBounded))
	}
	return append(lines, "Tariffs are caller-supplied assertions, not verified provider billing. Missing or stale prices block strict admission.")
}
func (m *Model) runCostBudget(name string, args []string) tea.Cmd {
	valid := len(args) == 0
	var amount int64
	if name == "/cost" && len(args) == 3 {
		var err error
		amount, err = parseCostAmount(args[1])
		valid = err == nil && len(args[0]) == 3 && strings.IndexFunc(args[0], func(r rune) bool { return r < 'A' || r > 'Z' }) < 0 && (args[2] == "strict" || args[2] == "advisory")
	}
	if !valid || m.controller == nil {
		m.appendNotice("Usage: /cost [CURRENCY TOTAL strict|advisory] | /prices. TOTAL is an absolute decimal ceiling.", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	controller := m.controller
	return m.startSessionOperation("cost-budget", func(ctx context.Context) sessionChangedMsg {
		if name == "/prices" {
			prices, err := controller.CostPrices(ctx)
			return sessionChangedMsg{preserveView: true, err: err, lines: priceLines(prices)}
		}
		if len(args) == 3 {
			if err := controller.DecideCostBudget(ctx, args[0], amount, args[2] == "strict"); err != nil {
				return sessionChangedMsg{preserveView: true, err: err}
			}
			return sessionChangedMsg{preserveView: true, lines: []string{fmt.Sprintf("Session monetary ceiling set to %s %s (%s). Existing charges remain. Use /cost to inspect.", args[0], costAmount(amount), args[2])}}
		}
		view, err := controller.CostBudget(ctx)
		return sessionChangedMsg{preserveView: true, err: err, lines: costBudgetLines(view)}
	})
}
