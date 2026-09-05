package tui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	teatest "github.com/charmbracelet/x/exp/teatest"
	"github.com/sausheong/hand/internal/agentio"
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

func TestModel_ToolCallShowsInputAndResultDetail(t *testing.T) {
	events := make(chan runtime.AgentEvent, 4)
	runner := &fakeRunner{events: events}
	m := NewModel(runner)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	m.BindProgram(tm.GetProgram())

	tm.Type("what does model.go do")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	events <- runtime.AgentEvent{
		Type:     runtime.EventToolCallStart,
		ToolCall: &llm.ToolCall{Name: "read_file", Input: json.RawMessage(`{"path":"internal/tui/model.go"}`)},
	}
	events <- runtime.AgentEvent{Type: runtime.EventToolResult, Result: &tool.ToolResult{Output: "package tui\n"}}
	events <- runtime.AgentEvent{Type: runtime.EventDone}
	close(events)

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "[tool: read_file] internal/tui/model.go") && contains(bts, "package tui")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestModel_ApprovalPromptBlocksAndRespondsYes(t *testing.T) {
	events := make(chan runtime.AgentEvent, 4)
	runner := &fakeRunner{events: events}
	m := NewModel(runner)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	m.BindProgram(tm.GetProgram())

	tm.Type("write a file")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	respond := make(chan agentio.Decision, 1)
	tm.Send(agentio.ApprovalRequest{
		Tool:    "write_file",
		Input:   json.RawMessage(`{"path":"x.txt"}`),
		Respond: respond,
	})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "Allow write_file?")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})

	select {
	case decision := <-respond:
		if decision != agentio.DecisionOnce {
			t.Fatalf("expected DecisionOnce after pressing y, got %v", decision)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Respond never received a value after pressing y")
	}

	events <- runtime.AgentEvent{Type: runtime.EventDone}
	close(events)

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "approved: write_file")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestModel_ApprovalPromptShowsPreview(t *testing.T) {
	events := make(chan runtime.AgentEvent, 4)
	runner := &fakeRunner{events: events}
	m := NewModel(runner)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	m.BindProgram(tm.GetProgram())

	tm.Type("run a command")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	respond := make(chan agentio.Decision, 1)
	tm.Send(agentio.ApprovalRequest{
		Tool:    "bash",
		Input:   json.RawMessage(`{"command":"go test ./..."}`),
		Preview: "$ go test ./...",
		Respond: respond,
	})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "$ go test ./...") && contains(bts, "Allow bash?")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	<-respond

	events <- runtime.AgentEvent{Type: runtime.EventDone}
	close(events)

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "ready")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestModel_ApprovalPromptRespondsNoOnAnyOtherKey(t *testing.T) {
	events := make(chan runtime.AgentEvent, 4)
	runner := &fakeRunner{events: events}
	m := NewModel(runner)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	m.BindProgram(tm.GetProgram())

	tm.Type("run rm -rf")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	respond := make(chan agentio.Decision, 1)
	tm.Send(agentio.ApprovalRequest{
		Tool:    "bash",
		Input:   json.RawMessage(`{"command":"rm -rf /"}`),
		Respond: respond,
	})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "Allow bash?")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	select {
	case decision := <-respond:
		if decision != agentio.DecisionDeny {
			t.Fatalf("expected DecisionDeny after pressing enter on a pending prompt, got %v", decision)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Respond never received a value")
	}

	events <- runtime.AgentEvent{Type: runtime.EventDone}
	close(events)

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "ready")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestModel_ApprovalPromptRespondsAlwaysOnA(t *testing.T) {
	events := make(chan runtime.AgentEvent, 4)
	runner := &fakeRunner{events: events}
	m := NewModel(runner)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	m.BindProgram(tm.GetProgram())

	tm.Type("write a file")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	respond := make(chan agentio.Decision, 1)
	tm.Send(agentio.ApprovalRequest{
		Tool:    "write_file",
		Input:   json.RawMessage(`{"path":"x.txt"}`),
		Respond: respond,
	})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "Allow write_file? [y]es / [a]lways / [n]o")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})

	select {
	case decision := <-respond:
		if decision != agentio.DecisionAlways {
			t.Fatalf("expected DecisionAlways after pressing a, got %v", decision)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Respond never received a value after pressing a")
	}

	events <- runtime.AgentEvent{Type: runtime.EventDone}
	close(events)

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "always allowed: write_file")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// The usage tests below call handleAgentEvent/handleCommand directly
// rather than driving a real tea.Program, for the same reason the
// slash-command dropdown tests in commands_test.go do: this is pure
// state-handling logic with no goroutines involved, so a direct
// synchronous call is both correct and simpler than teatest's async run
// loop.

func TestHandleAgentEvent_EventDoneCapturesUsage(t *testing.T) {
	m := NewModel(&fakeRunner{})
	usage := &llm.Usage{InputTokens: 100, OutputTokens: 20, CacheCreationInputTokens: 5, CacheReadInputTokens: 3}

	m.handleAgentEvent(runtime.AgentEvent{Type: runtime.EventDone, Usage: usage})

	if m.lastUsage != usage {
		t.Fatalf("lastUsage = %v, want %v", m.lastUsage, usage)
	}
}

func TestHandleAgentEvent_EventDoneWithNoUsageStaysNil(t *testing.T) {
	m := NewModel(&fakeRunner{})

	m.handleAgentEvent(runtime.AgentEvent{Type: runtime.EventDone})

	if m.lastUsage != nil {
		t.Fatalf("lastUsage = %v, want nil", m.lastUsage)
	}
}

func TestRunUsageCommand_NoUsageYet(t *testing.T) {
	m := NewModel(&fakeRunner{})

	m.handleCommand("/usage")

	if len(m.transcript) != 1 || !strings.Contains(m.transcript[0], "no usage recorded yet") {
		t.Fatalf("expected \"no usage recorded yet\" in transcript, got %v", m.transcript)
	}
}

func TestRunUsageCommand_WithUsage(t *testing.T) {
	m := NewModel(&fakeRunner{})
	m.lastUsage = &llm.Usage{InputTokens: 100, OutputTokens: 20, CacheCreationInputTokens: 5, CacheReadInputTokens: 3}

	m.handleCommand("/usage")

	want := "input: 100  output: 20  cache write: 5  cache read: 3"
	if len(m.transcript) != 1 || !strings.Contains(m.transcript[0], want) {
		t.Fatalf("expected %q in transcript, got %v", want, m.transcript)
	}
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
