package tui

import (
	"context"
	"errors"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/session"
	"strings"
	"testing"
)

func TestAssistantSourceReflowsAfterResize(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	source := "## Heading\n\n" + strings.Repeat("Markdown source should reflow from its original words. ", 8)
	m.streamBuf.WriteString(source)
	m.flushStream()
	wide := m.transcript[0]
	if m.sourceBlocks[0].Block.Text != source {
		t.Fatal("source discarded")
	}
	m.resize(35, 24)
	expected := renderMarkdown(source, 35, m.markdownStyle)
	if m.transcript[0] != expected || m.transcript[0] == wide {
		t.Fatal("resize reused stale rendered Markdown")
	}
	m.resize(80, 24)
	if m.transcript[0] != wide {
		t.Fatal("wide layout did not recover from source")
	}
	m.SetBanner("test", "test", "workspace")
	m.resize(40, 24)
	if m.sourceBlocks[4].Block.Text != source || m.transcript[4] != renderMarkdown(source, 40, m.markdownStyle) {
		t.Fatal("banner shifted source identity incorrectly")
	}
}

func TestReplayedAssistantRetainsSourceAndSanitises(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	source := "**bold**\x1b]52;c;payload\a " + strings.Repeat("more words ", 20)
	data := mustMarshal(t, session.MessageData{Text: source})
	m.LoadHistory([]session.SessionEntry{{Type: session.EntryTypeMessage, Role: "assistant", Data: data}})
	if m.sourceBlocks[0].Block.Text != source {
		t.Fatal("replay lost raw source")
	}
	m.resize(30, 24)
	if strings.Contains(m.transcript[0], "payload") {
		t.Fatal("reflow leaked terminal control payload")
	}
	if m.transcript[0] != renderMarkdown(sanitizeForTerminal(source), 30, m.markdownStyle) {
		t.Fatal("replay did not reflow from source")
	}
	m.replaceTranscript([]string{"replacement"}, nil)
	m.resize(50, 24)
	if len(m.sourceBlocks) != 1 || m.sourceBlocks[0].Block.Text != "replacement" {
		t.Fatal("stale source overwrote replacement")
	}
}

func TestApplicationToolOutputUsesTypedSourceAndViewer(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	raw := strings.Repeat("complete captured line\n", 100) + "last marker"
	m.renderApplicationEvent(app.Event{Kind: "tool_result", Details: app.Details{ResultPresent: true, Output: raw}})
	if len(m.toolOutputs) != 1 || m.toolOutputs[0].Output != raw || m.sourceBlocks[0].Block.Kind != "tool_result" {
		t.Fatal("application result missing typed source/viewer")
	}
	m.showOutput("")
	m.outputView.viewport.GotoBottom()
	if !strings.Contains(m.View(), "last marker") {
		t.Fatal("application full capture inaccessible")
	}
	replay := NewModel(nil, t.TempDir())
	replay.LoadHistory([]session.SessionEntry{session.ToolResultEntry("id", raw, "", nil)})
	if replay.sourceBlocks[0].Block != m.sourceBlocks[0].Block || replay.transcript[0] != m.transcript[0] {
		t.Fatal("application/replay differ")
	}
	m.renderApplicationEvent(app.Event{Kind: "tool_result", Details: app.Details{Output: "prefix", Truncated: true}})
	m.showOutput("")
	if !strings.Contains(m.outputView.block.text(), "truncated") || !strings.Contains(m.transcript[1], "truncated") {
		t.Fatal("truncated application details presented as complete output")
	}
}

func TestSessionTypedBlocksRetainCallAndCompactionData(t *testing.T) {
	entries := []session.SessionEntry{
		{Type: session.EntryTypeToolCall, Data: mustMarshal(t, session.ToolCallData{Tool: "bash", Input: []byte(`{"command":"echo hi"}`)})},
		{Type: session.EntryTypeCompaction, Data: mustMarshal(t, session.CompactionData{Summary: "retained summary", TurnsCompacted: 3})},
		{Type: session.EntryTypeMessage, Role: "user", Data: mustMarshal(t, session.MessageData{Text: "user input"})},
	}
	m := NewModel(nil, t.TempDir())
	m.LoadHistory(entries)
	if m.sourceBlocks[0].Block.Detail != `{"command":"echo hi"}` || m.sourceBlocks[1].Block.Text != "retained summary" || m.sourceBlocks[2].Block.Kind != "user" {
		t.Fatal("typed replay lost source fields")
	}
	m.resize(30, 24)
	if !strings.Contains(m.transcript[0], "echo hi") || !strings.Contains(m.transcript[1], "3 turns") {
		t.Fatal("typed replay lost presentation")
	}
}

func TestApprovalAndCompactionSourceState(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.renderApplicationEvent(app.Event{Kind: "approval_required", ApprovalID: "approval-1", Text: "preview", Details: app.Details{ToolName: "write_file", ToolInput: `{"path":"file.go"}`}})
	block := m.sourceBlocks[0].Block
	if block.Kind != "approval" || block.State != "pending" || block.ID != "approval-1" || block.Detail != `{"path":"file.go"}` || block.Preview != "preview" {
		t.Fatal("approval source lost", block)
	}
	m.recordApproval("denied")
	if m.sourceBlocks[1].Block.State != "denied" {
		t.Fatal("decision not typed")
	}
	m.renderApplicationEvent(app.Event{Kind: "compaction_done", Details: app.Details{Summary: "retained summary", TurnsCompacted: 4, TokensBefore: 5000, TokensAfter: 1000}})
	block = m.sourceBlocks[2].Block
	if block.Kind != "compaction" || block.Text != "retained summary" || block.TokensBefore != 5000 || block.TokensAfter != 1000 {
		t.Fatal("compaction source lost", block)
	}
	m.resize(35, 24)
	if !strings.Contains(m.transcript[1], "denied") || !strings.Contains(m.transcript[2], "5000") {
		t.Fatal("state lost on resize")
	}
}

func TestTypedRuntimeErrorSanitisesTerminalPayload(t *testing.T) {
	cause := errors.New("failed\x1b]52;c;payload\a")
	backend := applicationBackend{run: func(context.Context, string) (<-chan app.BackendEvent, error) {
		events := make(chan app.BackendEvent, 1)
		events <- app.BackendEvent{Err: cause}
		close(events)
		return events, nil
	}}
	m := applicationModel(t, app.New(backend, app.Options{MaxIterations: 1}), t.TempDir())
	driveApplication(t, m, m.startRun("work"))
	if m.lastOutcome == nil || m.lastOutcome.Status != agentio.InfrastructureError || !errors.Is(m.lastOutcome.Cause, cause) {
		t.Fatal("error cause lost", m.lastOutcome)
	}
	retained := false
	for _, source := range m.sourceBlocks {
		if strings.Contains(source.Block.Text, "payload") {
			retained = true
		}
	}
	if !retained {
		t.Fatal("error source not retained")
	}
	if strings.Contains(strings.Join(m.transcript, "\n"), "payload") {
		t.Fatal("unsafe error rendered")
	}
}
