package tui

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/app"
	"strconv"
	"strings"
)

func summarizerLines(view app.SummarizerView) []string {
	if view.Options.FollowMain {
		view.Options.Profile = "main model (following)"
	}
	return []string{fmt.Sprintf("Summariser: %s (%s/%s)", sanitizeForTerminal(view.Options.Profile), sanitizeForTerminal(view.Provider), sanitizeForTerminal(view.Model)), "Destination: " + sanitizeForTerminal(view.Destination), "Credential reference: " + sanitizeForTerminal(view.CredentialReference), fmt.Sprintf("Per attempt: %d output tokens; %d seconds", view.Options.MaxOutputTokens, view.Options.TimeoutSeconds), view.Disclosure}
}
func (m *Model) runSummarizer(args []string, follow ...bool) tea.Cmd {
	options := app.SummarizerOptions{}
	if len(follow) > 0 {
		options.FollowMain = follow[0]
	}
	valid := len(args) == 0 || len(args) == 1 || len(args) == 3
	if len(args) > 0 {
		options.Profile = args[0]
	}
	if len(args) == 3 {
		var err error
		options.MaxOutputTokens, err = strconv.Atoi(args[1])
		if err != nil {
			valid = false
		}
		options.TimeoutSeconds, err = strconv.Atoi(args[2])
		if err != nil {
			valid = false
		}
	}
	if !valid || m.controller == nil {
		m.appendNotice("Usage: /summarizer [PROFILE [OUTPUT_TOKENS TIMEOUT_SECONDS]]", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	controller := m.controller
	return m.startSessionOperation("summarizer-review", func(ctx context.Context) sessionChangedMsg {
		if len(args) == 0 && !options.FollowMain {
			view, ok := controller.SelectedSummarizer()
			if ok || view.Options.FollowMain {
				return sessionChangedMsg{preserveView: true, lines: summarizerLines(view)}
			}
			return sessionChangedMsg{preserveView: true, lines: []string{"No independent summariser selected. Available profiles: " + sanitizeForTerminal(strings.Join(controller.ProfileNames(), ", "))}}
		}
		view, err := controller.ReviewSummarizer(ctx, options)
		if err != nil {
			return sessionChangedMsg{preserveView: true, err: err}
		}
		lines := append(summarizerLines(view), "To select this destination: /summarizer-confirm "+view.Digest)
		return sessionChangedMsg{preserveView: true, summarizerReview: &view, lines: lines}
	})
}
func (m *Model) runSummarizerConfirm(args []string) tea.Cmd {
	if len(args) != 1 || m.summarizerReview == nil || m.controller == nil || args[0] != m.summarizerReview.Digest {
		m.appendNotice("Review /summarizer PROFILE first, then use /summarizer-confirm DIGEST.", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	view := *m.summarizerReview
	m.summarizerReview = nil
	controller := m.controller
	return m.startSessionOperation("summarizer-select", func(ctx context.Context) sessionChangedMsg {
		if err := controller.SelectSummarizer(ctx, view.Options, view.Digest); err != nil {
			return sessionChangedMsg{preserveView: true, err: err}
		}
		message := "Independent summariser selected for this process. Main-model changes will preserve this selection."
		if view.Options.FollowMain {
			message = "Summariser now follows the main model for this process, using the reviewed limits."
		}
		return sessionChangedMsg{preserveView: true, lines: []string{message}}
	})
}
