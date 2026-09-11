package tui

import (
	"encoding/json"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Keep commands readable without allowing long lines to fill the viewport.
func bashCommandPreview(raw string, width int) string {
	var input struct {
		Command string `json:"command"`
	}
	if json.Unmarshal([]byte(raw), &input) != nil || input.Command == "" {
		return ""
	}
	command := sanitizeForTerminal(strings.ReplaceAll(input.Command, "\r\n", "\n"))
	lines := strings.Split(strings.TrimRight(command, "\n"), "\n")
	more := len(lines) > 5
	if more {
		lines = lines[:5]
	}
	if width <= 0 {
		width = 80
	}
	for i, line := range lines {
		line = strings.ReplaceAll(line, "\t", "    ")
		lines[i] = "  " + ansi.Truncate(line, max(1, width-2), "…")
	}
	preview := strings.Join(lines, "\n")
	if more {
		preview += "\n  … (more lines)"
	}
	return preview
}
