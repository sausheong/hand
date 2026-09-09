package tui

import (
	"context"
	"encoding/json"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/app"
)

func (m *Model) runVerificationReview(args []string) tea.Cmd {
	if len(args) > 1 || m.controller == nil {
		m.appendNotice("Usage: /verify [profile]", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	selected := ""
	if len(args) == 1 {
		selected = args[0]
	}
	controller := m.controller
	return m.startSessionOperation("verify-review", func(ctx context.Context) sessionChangedMsg {
		profiles, err := controller.VerificationProfiles(ctx)
		if err != nil {
			return sessionChangedMsg{preserveView: true, err: err}
		}
		var shown []app.VerificationProfileView
		for _, p := range profiles {
			if selected == "" || p.Name == selected {
				shown = append(shown, p)
			}
		}
		if len(shown) == 0 {
			return sessionChangedMsg{preserveView: true, err: fmt.Errorf("verification profile not found")}
		}
		return sessionChangedMsg{preserveView: true, verificationReviews: shown, lines: verificationProfileLines(shown)}
	})
}
func verificationProfileLines(profiles []app.VerificationProfileView) []string {
	lines := []string{"Verification commands (review before confirming):"}
	for _, p := range profiles {
		argv, _ := json.Marshal(p.Command)
		lines = append(lines, sanitizeForTerminal(p.Name)+": "+sanitizeForTerminal(string(argv)), "Execution: "+sanitizeForTerminal(p.Boundary), "To run: /verify-confirm "+sanitizeForTerminal(p.Name)+" "+p.Digest)
	}
	return append(lines, "Commands may change files or external state; checkpoints do not undo external effects.")
}
func verificationResultLines(result app.VerificationResult) []string {
	lines := []string{fmt.Sprintf("Verification %s: %s (exit %d)", sanitizeForTerminal(result.Record.Profile), sanitizeForTerminal(result.Assessment.Status), result.Record.ExitCode)}
	if result.Assessment.Reason != "" {
		lines = append(lines, sanitizeForTerminal(result.Assessment.Reason))
	}
	if result.ID != "" {
		lines = append(lines, "Evidence: "+result.ID, "Recheck: /verify-check "+sanitizeForTerminal(result.Record.Profile)+" "+result.ID)
	}
	for _, item := range []struct{ name, text string }{{"stdout", result.Record.Stdout}, {"stderr", result.Record.Stderr}} {
		if item.text != "" {
			text := item.text
			if len(text) > 2048 {
				text = text[:2048] + "… (see saved evidence)"
			}
			lines = append(lines, item.name+": "+sanitizeForTerminal(text))
		}
	}
	if len(result.Record.Omissions) > 0 {
		lines = append(lines, fmt.Sprintf("Captured scope excludes %d paths.", len(result.Record.Omissions)))
	}
	return lines
}
func (m *Model) runVerificationConfirm(args []string) tea.Cmd {
	if len(args) == 2 && m.controller != nil {
		for _, p := range m.verificationReviews {
			if p.Name != args[0] || p.Digest != args[1] {
				continue
			}
			controller := m.controller
			name, digest := p.Name, p.Digest
			m.verificationReviews = nil
			return m.startSessionOperation("verify", func(ctx context.Context) sessionChangedMsg {
				result, err := controller.RunNamedVerification(ctx, name, digest)
				lines := []string{}
				if result.Record.Profile != "" {
					lines = verificationResultLines(result)
				}
				return sessionChangedMsg{preserveView: true, lines: lines, err: err}
			})
		}
	}
	m.appendNotice("Review /verify first, then use its exact /verify-confirm command.", "errorLineStyle")
	m.refreshViewport()
	return nil
}
func (m *Model) runVerificationCheck(args []string) tea.Cmd {
	if len(args) != 2 || m.controller == nil {
		m.appendNotice("Usage: /verify-check PROFILE EVIDENCE-ID", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	controller := m.controller
	name, id := args[0], args[1]
	return m.startSessionOperation("verify-check", func(ctx context.Context) sessionChangedMsg {
		result, err := controller.CheckSavedVerification(ctx, name, id)
		if err != nil {
			return sessionChangedMsg{preserveView: true, err: err}
		}
		return sessionChangedMsg{preserveView: true, lines: verificationResultLines(result)}
	})
}
