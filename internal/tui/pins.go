package tui

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/harness/runtime"
	"strings"
)

func (m *Model) runPins(name string, args []string) tea.Cmd {
	valid := name == "/pins" && len(args) == 0 || name == "/pin" && len(args) >= 3 || name == "/unpin" && len(args) == 1
	if !valid || m.controller == nil {
		m.appendNotice("Usage: /pins | /pin ID objective|constraint TEXT | /unpin ID", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	controller := m.controller
	return m.startSessionOperation("pins", func(ctx context.Context) sessionChangedMsg {
		var err error
		switch name {
		case "/pin":
			err = controller.SetContextPin(ctx, runtime.ContextPin{ID: args[0], Kind: args[1], Text: strings.Join(args[2:], " ")})
		case "/unpin":
			err = controller.RemoveContextPin(ctx, args[0])
		}
		if err != nil {
			return sessionChangedMsg{preserveView: true, err: err}
		}
		if name != "/pins" {
			return sessionChangedMsg{preserveView: true, lines: []string{"Session pin updated: " + args[0] + ". Use /pins to list the current set."}}
		}
		pins, err := controller.ContextPins(ctx)
		if err != nil {
			return sessionChangedMsg{preserveView: true, err: err}
		}
		lines := []string{fmt.Sprintf("Session pins: %d (apply across branches; survive compaction)", len(pins))}
		for _, pin := range pins {
			lines = append(lines, fmt.Sprintf("%s [%s]: %s", pin.Kind, pin.ID, pin.Text))
		}
		return sessionChangedMsg{preserveView: true, lines: lines}
	})
}
