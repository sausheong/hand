package tui

import "github.com/charmbracelet/lipgloss"

// Colors are deliberately muted and low-saturation — the vernacular of a
// terminal developer tool, not a chat app. Bold is reserved for the
// approval prompt alone: the one moment on screen that needs to grab
// attention over everything else.
var (
	colorSlate = lipgloss.Color("#6C7086") // structural/secondary: borders, tool-call lines, idle status
	colorUser  = lipgloss.Color("#89B4FA") // the "> " line the user typed, and the spinner
	colorGood  = lipgloss.Color("#A6E3A1") // tool success, approved
	colorBad   = lipgloss.Color("#F38BA8") // tool failure, errors, denied
	colorAlert = lipgloss.Color("#FAB387") // the approval prompt

	userLineStyle    = lipgloss.NewStyle().Foreground(colorUser).Bold(true)
	toolCallStyle    = lipgloss.NewStyle().Foreground(colorSlate)
	toolOKStyle      = lipgloss.NewStyle().Foreground(colorGood)
	toolErrStyle     = lipgloss.NewStyle().Foreground(colorBad)
	errorLineStyle   = lipgloss.NewStyle().Foreground(colorBad)
	approvedStyle    = lipgloss.NewStyle().Foreground(colorGood)
	deniedStyle      = lipgloss.NewStyle().Foreground(colorBad)
	statusIdleStyle  = lipgloss.NewStyle().Foreground(colorSlate)
	statusAlertStyle = lipgloss.NewStyle().Foreground(colorAlert).Bold(true)
	spinnerStyle     = lipgloss.NewStyle().Foreground(colorUser)
	inputBorderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorSlate)
)
