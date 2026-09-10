package tui

import (
	"context"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/sessionio"

	tea "github.com/charmbracelet/bubbletea"
)

type sessionChangedMsg struct {
	blocks              []TranscriptBlock
	evidenceIDs         []string
	verificationReviews []app.VerificationProfileView
	summarizerReview    *app.SummarizerView
	priceReview         *app.PriceReview
	recoveryPage        *app.CheckpointRecoveryPage
	restoreReview       *app.CheckpointRestorePreview
	sources             map[int]sourceLayout
	generation          uint64
	kind, id, warning   string
	lines               []string
	outputs             []ToolOutput
	err                 error
	preserveView        bool
	usage               sessionio.UsageSummary
}

func (m *Model) startSessionChange(kind string, change func(context.Context) error) tea.Cmd {
	m.abandonGoal("session_change_requested")
	controller := m.controller
	width, style := m.termWidth, m.markdownStyle
	return m.startSessionOperation(kind, func(ctx context.Context) sessionChangedMsg {
		result := sessionChangedMsg{kind: kind, err: change(ctx)}
		if result.err == nil {
			result.id = controller.SessionID()
			result.warning = controller.SessionWarning()
			var usageErr error
			result.usage, usageErr = controller.SessionUsage()
			if usageErr != nil {
				result.warning += " session usage unavailable: " + usageErr.Error()
			}
			// Once backend selection committed, cancellation must not restore an
			// obsolete UI identity. Only replay is optional at this point.
			for _, entry := range controller.SessionHistory() {
				if ctx.Err() != nil {
					result.warning += " history replay cancelled; session selection committed"
					break
				}
				if block, ok := outputFromEntry(entry); ok {
					block.SessionID = result.id
					result.outputs = append(result.outputs, block)
				}
				if line, ok := replayEntry(entry, width, style); ok {
					if block, ok := sessionBlock(entry); ok {
						if result.sources == nil {
							result.sources = make(map[int]sourceLayout)
						}
						result.sources[len(result.lines)] = sourceLayout{Block: block, Width: width, Style: style, Rendered: line}
					}
					result.lines = append(result.lines, line)
				}
			}
		}
		return result
	})
}

// Session metadata operations share cancellation and joining with selection,
// but retain the current identity, transcript and usage counters.
func (m *Model) startSessionOperation(kind string, operation func(context.Context) sessionChangedMsg) tea.Cmd {
	m.restoreReview = nil
	m.recoveryPage = nil
	m.verificationReviews = nil
	m.summarizerReview = nil
	m.priceReview = nil
	m.evidenceIDs = nil
	ctx, cancel := context.WithCancel(context.Background())
	m.sessionChanging = true
	m.sessionCancel = cancel
	m.quitAfterSession = false
	m.sessionGeneration++
	generation := m.sessionGeneration
	done := make(chan struct{})
	m.sessionDone = done
	results := make(chan sessionChangedMsg, 1)
	go func() {
		defer close(done)
		defer cancel()
		result := operation(ctx)
		result.generation = generation
		result.kind = kind
		results <- result
	}()
	m.appendNotice("session operation in progress; Ctrl+C cancels", "toolCallStyle")
	m.refreshViewport()
	return func() tea.Msg { return <-results }
}

func (m *Model) finishSessionChange(result sessionChangedMsg) tea.Cmd {
	if !m.sessionChanging || result.generation != m.sessionGeneration {
		return nil
	}
	// The result is sent before worker exit; join it before releasing UI guards.
	<-m.sessionDone
	m.sessionDone = nil
	m.sessionChanging = false
	m.sessionCancel = nil
	if result.err != nil {
		if result.preserveView {
			m.appendNotices(result.lines)
		}
		m.appendNotice("session operation failed: "+sanitizeForTerminal(result.err.Error()), "errorLineStyle")
	} else if result.preserveView {
		m.restoreReview = result.restoreReview
		m.recoveryPage = result.recoveryPage
		m.verificationReviews = result.verificationReviews
		m.summarizerReview = result.summarizerReview
		m.priceReview = result.priceReview
		m.evidenceIDs = result.evidenceIDs
		m.appendNotices(result.lines)
		for _, block := range result.blocks {
			m.appendSourceBlock(block)
		}
	} else {
		m.abandonGoal("session_changed")
		m.identity.SessionID = result.id
		m.identity.Generation++
		m.resetSessionView()
		m.sessionUsage, m.usageRequests, m.usageUnknown = result.usage.Total, result.usage.Requests, result.usage.Unknown
		m.usagePriorUnknown = result.usage.PriorUsageUnknown
		m.replaceTranscript(result.lines, result.sources)
		m.toolOutputs = result.outputs
		m.closeOutputView()
		text := "resumed session " + sanitizeForTerminal(result.id)
		if result.kind == "fork" {
			text = "forked selected history into a new session; source is saved"
		}
		if result.kind == "branch" {
			text = "selected session branch; other branches are saved"
		}
		if result.kind == "new" {
			text = "started a new session; previous history is saved"
		}
		m.appendNotice(text, "approvedStyle")
		if result.warning != "" {
			m.appendNotice(sanitizeForTerminal(result.warning), "errorLineStyle")
		}
	}
	m.refreshViewport()
	if m.quitAfterSession {
		return tea.Quit
	}
	return nil
}
