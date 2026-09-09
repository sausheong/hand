package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/harness/runtime"
)

// SetMCPStatus exposes construction outcomes after the initial banner.
func (m *Model) SetMCPStatus(status []runtime.MCPServerStatus) {
	for _, s := range status {
		text := fmt.Sprintf("MCP %s: %s (%d tools)", s.Name, s.State, s.Tools)
		if s.Optional {
			text += " [optional]"
		}
		m.appendNotice(sanitizeForTerminal(text), "toolCallStyle")
	}
	m.refreshViewport()
}

func (m *Model) runMCPCommand(args []string) tea.Cmd {
	if m.controller == nil || m.controller.OptionalMCP == nil {
		m.appendNotice("No optional MCP connections configured", "toolCallStyle")
		m.refreshViewport()
		return nil
	}
	manager := m.controller.OptionalMCP
	if len(args) == 2 && args[0] == "retry" {
		text := "MCP retry queued: " + args[1]
		if err := manager.Retry(args[1]); err != nil {
			text = err.Error()
		}
		m.appendNotice(sanitizeForTerminal(text), "toolCallStyle")
	} else if len(args) != 0 {
		m.appendNotice("Usage: /mcp or /mcp retry <server>", "toolCallStyle")
	} else {
		for _, s := range manager.Status() {
			m.appendNotice(sanitizeForTerminal("MCP "+s.Name+": "+s.State), "toolCallStyle")
		}
	}
	m.refreshViewport()
	return nil
}

// SetOptionalMCPStatus renders invocation startup state before Program.Run.
func (m *Model) SetOptionalMCPStatus() {
	m.runMCPCommand(nil)
	m.appendNotice("/mcp shows current connections; /mcp retry <server> retries a failure", "toolCallStyle")
	m.refreshViewport()
}
