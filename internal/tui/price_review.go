package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/app"
)

func (m *Model) runPriceReview(args []string) tea.Cmd {
	if len(args) == 0 || m.controller == nil {
		m.appendNotice("Usage: /prices-review PATH (versioned tariff JSON)", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	path := strings.Join(args, " ")
	return m.startSessionOperation("price-review", func(ctx context.Context) sessionChangedMsg {
		if err := ctx.Err(); err != nil {
			return sessionChangedMsg{preserveView: true, err: err}
		}
		review, err := app.ReadPriceReview(path)
		if err != nil {
			return sessionChangedMsg{preserveView: true, err: err}
		}
		lines := append(priceLines(review.Prices), fmt.Sprintf("This replaces the entire session tariff table with %d entries. Ceiling and prior charges remain unchanged.", len(review.Prices)), "Confirm the reviewed table: /prices-confirm "+review.Digest)
		return sessionChangedMsg{preserveView: true, priceReview: &review, lines: lines}
	})
}
func (m *Model) runPriceConfirm(args []string) tea.Cmd {
	if len(args) != 1 || m.priceReview == nil || m.controller == nil || args[0] != m.priceReview.Digest {
		m.appendNotice("Use /prices-review PATH, then /prices-confirm DIGEST from that review.", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	review := *m.priceReview
	m.priceReview = nil
	controller := m.controller
	return m.startSessionOperation("price-install", func(ctx context.Context) sessionChangedMsg {
		if err := controller.ApplyPriceReview(ctx, review); err != nil {
			return sessionChangedMsg{preserveView: true, err: err}
		}
		return sessionChangedMsg{preserveView: true, lines: []string{fmt.Sprintf("Installed %d reviewed session tariffs. Use /prices to inspect.", len(review.Prices))}}
	})
}
