package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// commandDef backs both /help's listing and the auto-complete dropdown
// shown while typing a command name, so the two can't drift apart.
type commandDef struct {
	name string
	desc string
}

var commandDefs = []commandDef{
	{"/help", "show this message"},
	{"/model", "show the active model, or /model <name> to switch"},
	{"/new", "discard this workspace's saved session and start fresh"},
	{"/clear", "clear the on-screen transcript (keeps the saved session)"},
	{"/compact", "force a context-compaction pass now"},
	{"/usage", "show token usage from the most recent turn"},
	{"/exit", "quit hand"},
}

func buildHelpText() string {
	var b strings.Builder
	b.WriteString("Commands:\n")
	for i, cd := range commandDefs {
		fmt.Fprintf(&b, "  %-10s %s", cd.name, cd.desc)
		if i < len(commandDefs)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

var helpText = buildHelpText()

// isCommand reports whether text should be dispatched as a slash command
// rather than sent to the agent as a user message.
func isCommand(text string) bool {
	return strings.HasPrefix(text, "/")
}

// matchingCommands returns the known commands whose name starts with
// prefix, for the auto-complete dropdown. It only matches while the user
// is still typing the command word itself — once whitespace appears
// (moving into arguments, e.g. "/model anthropic/..."), there's nothing
// left to complete.
func matchingCommands(prefix string) []commandDef {
	if !strings.HasPrefix(prefix, "/") || strings.ContainsAny(prefix, " \t") {
		return nil
	}
	var out []commandDef
	for _, cd := range commandDefs {
		if strings.HasPrefix(cd.name, prefix) {
			out = append(out, cd)
		}
	}
	return out
}

// commandSuggestions returns the auto-complete matches for the text
// currently in the input box, or nil when there's nothing to suggest
// (empty input, not a command, mid-argument, or a turn/approval is
// already in progress and the input isn't being used for a command).
func (m *Model) commandSuggestions() []commandDef {
	if m.pending != nil || m.running {
		return nil
	}
	return matchingCommands(m.textarea.Value())
}

// renderSuggestions draws the auto-complete dropdown, highlighting the
// currently selected entry (see suggestIndex, moved by the up/down keys
// in handleKey).
func (m *Model) renderSuggestions(suggestions []commandDef) string {
	if m.suggestIndex < 0 || m.suggestIndex >= len(suggestions) {
		m.suggestIndex = 0
	}
	lines := make([]string, len(suggestions))
	for i, cd := range suggestions {
		line := fmt.Sprintf("%-10s %s", cd.name, cd.desc)
		if i == m.suggestIndex {
			lines[i] = userLineStyle.Render("› " + line)
		} else {
			lines[i] = toolCallStyle.Render("  " + line)
		}
	}
	return strings.Join(lines, "\n")
}

// completeSuggestion fills the input with the selected suggestion's full
// command name (Tab), leaving the cursor ready for arguments.
func (m *Model) completeSuggestion(suggestions []commandDef) {
	if m.suggestIndex < 0 || m.suggestIndex >= len(suggestions) {
		m.suggestIndex = 0
	}
	m.textarea.SetValue(suggestions[m.suggestIndex].name + " ")
	m.textarea.CursorEnd()
	m.suggestIndex = 0
}

// handleCommand parses and runs a slash command, appending its result to
// the transcript. It never starts an agent turn.
func (m *Model) handleCommand(text string) tea.Cmd {
	fields := strings.Fields(text)
	name, args := fields[0], fields[1:]

	switch name {
	case "/exit", "/quit":
		return tea.Quit

	case "/help":
		m.transcript = append(m.transcript, toolCallStyle.Render(helpText))

	case "/clear":
		m.transcript = nil
		m.streamBuf.Reset()

	case "/model":
		m.runModelCommand(args)

	case "/new":
		m.runNewCommand()

	case "/compact":
		m.runCompactCommand()

	case "/usage":
		m.runUsageCommand()

	default:
		m.transcript = append(m.transcript, errorLineStyle.Render("unknown command: "+name+" (try /help)"))
	}

	m.refreshViewport()
	return nil
}

func (m *Model) runModelCommand(args []string) {
	if m.controller == nil {
		m.transcript = append(m.transcript, errorLineStyle.Render("/model is not available in this build"))
		return
	}
	if len(args) == 0 {
		m.transcript = append(m.transcript, toolCallStyle.Render("model: "+m.controller.CurrentModel()))
		return
	}
	target := args[0]
	if err := m.controller.SwitchModel(target); err != nil {
		m.transcript = append(m.transcript, errorLineStyle.Render("model switch failed: "+err.Error()))
		return
	}
	m.setModel(m.controller.CurrentModel())
	m.transcript = append(m.transcript, approvedStyle.Render("model switched to "+m.model))
}

func (m *Model) runNewCommand() {
	if m.controller == nil {
		m.transcript = append(m.transcript, errorLineStyle.Render("/new is not available in this build"))
		return
	}
	if err := m.controller.NewSession(); err != nil {
		m.transcript = append(m.transcript, errorLineStyle.Render("new session failed: "+err.Error()))
		return
	}
	m.transcript = nil
	m.streamBuf.Reset()
	m.transcript = append(m.transcript, approvedStyle.Render("started a new session"))
}

func (m *Model) runCompactCommand() {
	if m.controller == nil {
		m.transcript = append(m.transcript, errorLineStyle.Render("/compact is not available in this build"))
		return
	}
	result, err := m.controller.Compact(context.Background())
	if err != nil {
		m.transcript = append(m.transcript, errorLineStyle.Render("compact failed: "+err.Error()))
		return
	}
	if !result.Compacted {
		reason := result.Skipped
		if reason == "" {
			reason = "unknown"
		}
		m.transcript = append(m.transcript, toolCallStyle.Render("compact skipped: "+reason))
		return
	}
	m.transcript = append(m.transcript, approvedStyle.Render(fmt.Sprintf("compacted %d turns", result.TurnsCompacted)))
}

func (m *Model) runUsageCommand() {
	if m.lastUsage == nil {
		m.transcript = append(m.transcript, toolCallStyle.Render("no usage recorded yet"))
		return
	}
	u := m.lastUsage
	lines := []string{
		fmt.Sprintf("input: %d  output: %d  cache write: %d  cache read: %d",
			u.InputTokens, u.OutputTokens, u.CacheCreationInputTokens, u.CacheReadInputTokens),
		fmt.Sprintf("last turn: %s tok in %s", formatTokenCount(totalTokens(*u)), formatDuration(m.lastTurnDuration)),
		fmt.Sprintf("session total: %s tok", formatTokenCount(totalTokens(m.sessionUsage))),
		fmt.Sprintf("context: %s", m.contextSummary()),
	}
	m.transcript = append(m.transcript, toolCallStyle.Render(strings.Join(lines, "\n")))
}
