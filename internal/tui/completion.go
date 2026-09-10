package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/agentio"
	"strings"
	"unicode/utf8"
)

type referenceCompletionMsg struct {
	text, workspace string
	cursor          int
	result          agentio.ReferenceCompletion
	err             error
}

func (m *Model) inputCursor() int {
	lines := strings.Split(m.textarea.Value(), "\n")
	row := m.textarea.Line()
	if row >= len(lines) {
		return len(m.textarea.Value())
	}
	info := m.textarea.LineInfo()
	column := info.StartColumn + info.ColumnOffset
	runes := []rune(lines[row])
	if column > len(runes) {
		column = len(runes)
	}
	offset := 0
	for i := 0; i < row; i++ {
		offset += len(lines[i]) + 1
	}
	return offset + len(string(runes[:column]))
}
func (m *Model) completeReference() tea.Cmd {
	text, workspace, cursor := m.textarea.Value(), m.workspace, m.inputCursor()
	return func() tea.Msg {
		result, err := agentio.CompleteReference(context.Background(), workspace, text, cursor)
		return referenceCompletionMsg{text: text, workspace: workspace, cursor: cursor, result: result, err: err}
	}
}
func (m *Model) applyReferenceCompletion(msg referenceCompletionMsg) {
	if m.editorActive || m.textarea.Value() != msg.text || m.workspace != msg.workspace || m.inputCursor() != msg.cursor {
		return
	}
	if msg.err != nil {
		m.queueMessage("File completion: " + msg.err.Error())
		return
	}
	if len(msg.result.Candidates) == 0 {
		return
	}
	if len(msg.result.Candidates) > 1 {
		m.queueMessage(strings.Join(msg.result.Candidates, "\n"))
		return
	}
	replacement := msg.result.Candidates[0]
	text := msg.text[:msg.result.Start] + replacement + msg.text[msg.result.End:]
	m.textarea.SetValue(text)
	if m.textarea.Value() != text {
		m.textarea.SetValue(msg.text)
		m.queueMessage("Completion could not fit the input; draft preserved.")
		return
	}
	before := text[:msg.result.Start+len(replacement)]
	row := strings.Count(before, "\n")
	column := utf8.RuneCountInString(before[strings.LastIndex(before, "\n")+1:])
	for m.textarea.Line() > row {
		m.textarea.CursorUp()
	}
	m.textarea.SetCursor(column)
}
