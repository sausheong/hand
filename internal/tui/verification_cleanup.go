package tui

import (
	"context"
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/verification"
)

func evidencePageLines(page verification.RecordPage) []string {
	lines := []string{fmt.Sprintf("Saved verification evidence: %d records", page.Total)}
	for _, id := range page.IDs {
		lines = append(lines, id)
	}
	if page.Next < page.Total {
		lines = append(lines, fmt.Sprintf("Next page: /verify-list %d", page.Next))
	}
	return append(lines, "IDs alone do not prove tests passed. Use /verify-check PROFILE ID to assess.", "To delete one listed record: /verify-delete ID confirm. Referenced snapshots and output remain.")
}
func (m *Model) runVerificationList(args []string) tea.Cmd {
	offset := 0
	if len(args) == 1 {
		var err error
		offset, err = strconv.Atoi(args[0])
		if err != nil {
			offset = -1
		}
	}
	if len(args) > 1 || offset < 0 || m.controller == nil {
		m.appendNotice("Usage: /verify-list [offset]", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	controller := m.controller
	return m.startSessionOperation("verify-list", func(ctx context.Context) sessionChangedMsg {
		page, err := controller.ListVerificationEvidence(ctx, offset)
		if err != nil {
			return sessionChangedMsg{preserveView: true, err: err}
		}
		return sessionChangedMsg{preserveView: true, evidenceIDs: page.IDs, lines: evidencePageLines(page)}
	})
}
func (m *Model) runVerificationDelete(args []string) tea.Cmd {
	if len(args) == 2 && args[1] == "confirm" && m.controller != nil {
		for _, id := range m.evidenceIDs {
			if id != args[0] {
				continue
			}
			controller := m.controller
			selected := id
			m.evidenceIDs = nil
			return m.startSessionOperation("verify-delete", func(ctx context.Context) sessionChangedMsg {
				err := controller.DeleteVerificationEvidence(ctx, selected)
				if err != nil {
					return sessionChangedMsg{preserveView: true, err: err}
				}
				return sessionChangedMsg{preserveView: true, lines: []string{"Deleted verification record " + selected + ". Referenced snapshots and output retained."}}
			})
		}
	}
	m.appendNotice("List /verify-list first, then use /verify-delete LISTED-ID confirm.", "errorLineStyle")
	m.refreshViewport()
	return nil
}
