package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/hand/internal/extensions"
)

type extensionQuestionTick struct{ generation uint64 }

func extensionTick(generation uint64) tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return extensionQuestionTick{generation} })
}
func (m *Model) runExtension(args []string) tea.Cmd {
	if m.controller == nil || m.controller.Extensions == nil || len(args) < 2 {
		m.appendNotice("Usage: /extension NAME COMMAND [arguments] (requires reviewed extensions)", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	host := m.controller.Extensions
	command := m.startSessionOperation("extension", func(ctx context.Context) sessionChangedMsg {
		presentation, err := host.Execute(ctx, args[0], args[1], strings.Join(args[2:], " "))
		result := sessionChangedMsg{preserveView: true, err: err}
		if err != nil {
			return result
		}
		result.blocks, result.err = extensionPresentationBlocks(args[0], presentation)
		return result
	})
	if command == nil {
		return nil
	}
	return tea.Batch(command, m.startExtensionPolling())
}
func (m *Model) pollExtensionQuestion(tick extensionQuestionTick) tea.Cmd {
	if !(m.sessionChanging || m.running || m.compacting) || tick.generation != m.extensionGeneration || m.controller == nil || m.controller.Extensions == nil {
		return nil
	}
	pending := m.controller.Extensions.Pending()
	if len(pending) > 0 && (m.extensionQuestion == nil || m.extensionQuestion.Token != pending[0].Token) {
		question := pending[0]
		m.dismissExtensionQuestion()
		m.extensionDraft = m.textarea.Value()
		m.textarea.Reset()
		m.extensionQuestion = &question
		m.appendNotice("Extension "+sanitizeForTerminal(question.Extension)+": "+sanitizeForTerminal(question.Question.Title), "toolCallStyle")
		for _, option := range question.Question.Options {
			m.appendNotice(fmt.Sprintf("%s: %s", sanitizeForTerminal(option.ID), sanitizeForTerminal(option.Label)), "toolCallStyle")
		}
		hint := "Enter an option ID; Esc cancels this question; Ctrl+C cancels the command."
		if question.Question.AllowFreeText {
			hint = "Enter your answer; use choice:ID for a listed option. Esc cancels this question; Ctrl+C cancels the command."
		}
		m.appendNotice(hint, "toolCallStyle")
		m.refreshViewport()
	}
	if len(pending) == 0 {
		m.dismissExtensionQuestion()
	}
	return extensionTick(tick.generation)
}
func extensionAnswer(q extensions.PendingQuestion, text string, cancelled bool) protocol.Answer {
	answer := protocol.Answer{ID: q.Question.ID, Cancelled: cancelled}
	if cancelled {
		return answer
	}
	if !q.Question.AllowFreeText {
		answer.Choice = text
	} else if strings.HasPrefix(text, "choice:") {
		answer.Choice = strings.TrimPrefix(text, "choice:")
	} else {
		answer.Text = text
	}
	return answer
}
func (m *Model) answerExtension(text string, cancelled bool) {
	q := m.extensionQuestion
	if q == nil {
		return
	}
	if err := m.controller.Extensions.Answer(q.Token, extensionAnswer(*q, text, cancelled)); err != nil {
		m.appendNotice("Extension answer rejected: "+sanitizeForTerminal(err.Error()), "errorLineStyle")
	} else {
		m.dismissExtensionQuestion()
	}
	m.refreshViewport()
}

func (m *Model) startExtensionPolling() tea.Cmd {
	if m.controller == nil || m.controller.Extensions == nil || !(m.sessionChanging || m.running || m.compacting) {
		return nil
	}
	m.dismissExtensionQuestion()
	m.extensionGeneration++
	return extensionTick(m.extensionGeneration)
}
func (m *Model) dismissExtensionQuestion() {
	if m.extensionQuestion != nil {
		m.textarea.SetValue(m.extensionDraft)
		m.extensionDraft = ""
		m.extensionQuestion = nil
	}
}
