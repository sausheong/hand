package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/llm/llmtest"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
)

type contextSequenceProvider struct {
	llmtest.Base
	calls       int
	unknownLast bool
}

func (p *contextSequenceProvider) ChatStream(_ context.Context, req llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	p.calls++
	events := make(chan llm.ChatEvent, 2)
	if p.calls < 10 {
		events <- llm.ChatEvent{Type: llm.EventToolCallDone, ToolCall: &llm.ToolCall{ID: fmt.Sprint(p.calls), Name: "noop", Input: json.RawMessage(`{}`)}}
	}
	usage := &llm.Usage{InputTokens: 20000, OutputTokens: 100, CacheReadInputTokens: 15000}
	if p.calls == 10 && p.unknownLast {
		usage = nil
	}
	if p.calls >= 10 {
		events <- llm.ChatEvent{Type: llm.EventTextDelta, Text: "Answer"}
	}
	events <- llm.ChatEvent{Type: llm.EventDone, Usage: usage}
	close(events)
	return events, nil
}

type contextSequenceTools struct{ calls int }

func (p *contextSequenceTools) Execute(context.Context, string, json.RawMessage) (tool.ToolResult, error) {
	p.calls++
	return tool.ToolResult{Output: "ok"}, nil
}
func (*contextSequenceTools) ToolDefs() []llm.ToolDef      { return []llm.ToolDef{{Name: "noop"}} }
func (*contextSequenceTools) Names() []string              { return []string{"noop"} }
func (*contextSequenceTools) Get(string) (tool.Tool, bool) { return nil, false }

func TestRuntimeRequestsReachTUIContextWithoutCumulativeInflation(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(fmt.Sprint("unknown_last_", unknown), func(t *testing.T) {
			store := session.NewStore(t.TempDir())
			if err := store.Create("hand", "key"); err != nil {
				t.Fatal(err)
			}
			sess, err := store.LoadExclusive("hand", "key")
			if err != nil {
				t.Fatal(err)
			}
			defer sess.Close()
			provider := &contextSequenceProvider{unknownLast: unknown}
			tools := &contextSequenceTools{}
			rt := &runtime.Runtime{LLM: provider, Session: sess, Tools: tools, AgentID: "hand", Model: "test", MaxTurns: 10}
			service := app.New(&app.HarnessBackend{Runtime: rt}, app.Options{SessionID: sess.ID, MaxIterations: 1})
			m := applicationModel(t, service, t.TempDir())
			defer m.CloseApplication()
			m.SetContextLimit(100000)
			driveApplication(t, m, m.startRun("go"))
			if provider.calls != 10 || tools.calls != 9 || m.running || m.lastOutcome == nil || m.lastOutcome.Status != agentio.Completed {
				t.Fatal("runtime journey incomplete", provider.calls, tools.calls, m.lastOutcome)
			}
			expected := 201000
			unknownCount := 0
			if unknown {
				expected = 180900
				unknownCount = 1
				if !strings.Contains(m.contextSummary(), "unknown") {
					t.Fatal("missing last usage retained context", m.contextSummary())
				}
			} else if !strings.Contains(m.contextSummary(), "ctx 20k/100k (20%)") {
				t.Fatal("wrong final request context", m.contextSummary())
			}
			if totalTokens(m.sessionUsage) != expected || m.usageRequests != 10 || m.usageUnknown != unknownCount {
				t.Fatal("TUI totals differ from actual attempts", m.sessionUsage, m.usageRequests, m.usageUnknown)
			}
			if err := sess.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := store.LoadExclusive("hand", "key")
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			summary, err := sessionio.ReadUsage(reopened)
			if err != nil || totalTokens(summary.Total) != expected || summary.Requests != 10 || summary.Unknown != unknownCount {
				t.Fatal("persisted accounting differs from display", summary, err)
			}
		})
	}
}
