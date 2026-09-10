package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) runTreeCommand(args []string) tea.Cmd {
	if m.controller == nil {
		m.appendNotice("workspace sessions unavailable", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	if len(args) > 1 {
		m.appendNotice("usage: /tree [entry ID]", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	controller := m.controller
	if len(args) == 1 {
		return m.startSessionChange("branch", func(ctx context.Context) error { return controller.SelectSessionNode(ctx, args[0]) })
	}
	return m.startSessionOperation("tree", func(ctx context.Context) sessionChangedMsg {
		result := sessionChangedMsg{preserveView: true}
		nodes, err := controller.SessionTree(ctx)
		if err != nil {
			result.err = err
			return result
		}
		if len(nodes) == 0 {
			result.lines = []string{toolCallStyle.Render("session has no conversation nodes")}
			return result
		}
		result.lines = append(result.lines, toolCallStyle.Render("session tree (* selected; use /tree <entry ID> to continue from a node)"))
		for _, node := range nodes {
			if err := ctx.Err(); err != nil {
				result.err = err
				return result
			}
			marker := " "
			if node.Selected {
				marker = "*"
			}
			parent := node.ParentID
			if parent == "" {
				parent = "root"
			}
			result.lines = append(result.lines, toolCallStyle.Render(sanitizeForTerminal(fmt.Sprintf("%s %s <- %s  %s %s", marker, node.ID, parent, node.Type, node.Role))))
		}
		return result
	})
}
