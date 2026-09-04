package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	teatest "github.com/charmbracelet/x/exp/teatest"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
)

type fakeRunner struct {
	events chan runtime.AgentEvent
	err    error
}

func (f *fakeRunner) Run(ctx context.Context, userMsg string, images []llm.ImageContent) (<-chan runtime.AgentEvent, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.events, nil
}

var toolResultOK = tool.ToolResult{Output: "ok"}

func TestModel_StreamsAssistantTextIntoTranscript(t *testing.T) {
	events := make(chan runtime.AgentEvent, 4)
	runner := &fakeRunner{events: events}
	m := NewModel(runner)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	m.BindProgram(tm.GetProgram())

	tm.Type("hello there")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	events <- runtime.AgentEvent{Type: runtime.EventTextDelta, Text: "Hi"}
	events <- runtime.AgentEvent{Type: runtime.EventTextDelta, Text: " back"}
	events <- runtime.AgentEvent{Type: runtime.EventDone}
	close(events)

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "Hi back") && contains(bts, "ready")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestModel_ToolCallAndResultRender(t *testing.T) {
	events := make(chan runtime.AgentEvent, 4)
	runner := &fakeRunner{events: events}
	m := NewModel(runner)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	m.BindProgram(tm.GetProgram())

	tm.Type("run a build")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	events <- runtime.AgentEvent{Type: runtime.EventToolCallStart, ToolCall: &llm.ToolCall{Name: "read_file"}}
	events <- runtime.AgentEvent{Type: runtime.EventToolResult, Result: &toolResultOK}
	events <- runtime.AgentEvent{Type: runtime.EventDone}
	close(events)

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "[tool: read_file]") && contains(bts, "✓")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func contains(haystack []byte, needle string) bool {
	return len(needle) == 0 || indexOf(string(haystack), needle) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
