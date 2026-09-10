package app

import (
	"context"
	"encoding/json"
	"github.com/sausheong/hand/internal/agentio"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sausheong/harness/compaction"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
)

func TestHarnessDetailsAreIndependentSnapshots(t *testing.T) {
	input := json.RawMessage(`{"path":"original"}`)
	call := &llm.ToolCall{ID: "call-1", Name: "read_file", Input: input}
	metadata := map[string]any{"path": "original"}
	result := &tool.ToolResult{Output: "contents", Error: "warning", Metadata: metadata, Images: []llm.ImageContent{{Data: []byte{1}}}}
	event, ok := translateHarnessEvent(runtime.AgentEvent{Type: runtime.EventToolResult, ToolCall: call, Result: result})
	if !ok || event.Kind != "tool_result" {
		t.Fatal(event)
	}
	input[2] = 'X'
	call.Name = "changed"
	result.Output = "changed"
	metadata["path"] = "changed"
	d := event.Details
	if d.ToolName != "read_file" || d.ToolInput != `{"path":"original"}` || d.Output != "contents" || d.Metadata != `{"path":"original"}` || d.ImageCount != 1 || !d.ToolPresent || !d.ResultPresent {
		t.Fatalf("snapshot changed: %+v", d)
	}
}

func TestHarnessUsageAndCompactionDetails(t *testing.T) {
	u := &llm.Usage{InputTokens: 12, OutputTokens: 3, CacheCreationInputTokens: 4, CacheReadInputTokens: 5}
	e, _ := translateHarnessEvent(runtime.AgentEvent{Type: runtime.EventDone, Usage: u})
	u.InputTokens = 999
	if !e.Done || e.Kind != "usage" || !e.Details.UsageKnown || e.Details.InputTokens != 12 || e.Details.OutputTokens != 3 || e.Details.CacheCreationInputTokens != 4 || e.Details.CacheReadInputTokens != 5 {
		t.Fatal(e)
	}
	missing, _ := translateHarnessEvent(runtime.AgentEvent{Type: runtime.EventDone})
	if missing.Details.UsageKnown {
		t.Fatal("missing usage became known zero")
	}
	c := &compaction.Result{Compacted: true, Summary: "summary", TokensBefore: 100, TokensAfter: 20, TurnsCompacted: 4, DurationMs: 7}
	for kind, name := range map[runtime.EventType]string{runtime.EventCompactionStart: "compaction_start", runtime.EventCompactionDone: "compaction_done", runtime.EventCompactionSkipped: "compaction_skipped"} {
		e, _ := translateHarnessEvent(runtime.AgentEvent{Type: kind, Compaction: c})
		if e.Kind != name || !e.Details.CompactionPresent || !e.Details.Compacted || e.Details.Summary != "summary" || e.Details.TokensBefore != 100 || e.Details.TokensAfter != 20 || e.Details.TurnsCompacted != 4 || e.Details.DurationMs != 7 {
			t.Fatal(e)
		}
	}
}

func TestDetailsAggregateBound(t *testing.T) {
	d := boundDetails(Details{ToolID: "id", ToolName: "tool", ToolInput: strings.Repeat("界", MaxEventTextBytes), Output: strings.Repeat("x", MaxEventTextBytes), Metadata: "{}", Summary: "summary"})
	size := 0
	for _, s := range []string{d.ToolID, d.ToolName, d.ToolInput, d.Output, d.ToolError, d.Metadata, d.CompactionReason, d.Skipped, d.Summary} {
		size += len(s)
		if !utf8.ValidString(s) {
			t.Fatal("split Unicode")
		}
	}
	if size > MaxEventTextBytes || !d.Truncated {
		t.Fatal(size, d.Truncated)
	}
}

func TestHarnessMalformedEvents(t *testing.T) {
	for _, kind := range []runtime.EventType{runtime.EventError, runtime.EventAborted} {
		e, ok := translateHarnessEvent(runtime.AgentEvent{Type: kind})
		if !ok || e.Err == nil {
			t.Fatal(e)
		}
	}
	if _, ok := translateHarnessEvent(runtime.AgentEvent{Type: runtime.EventType(999)}); ok {
		t.Fatal("unknown event accepted")
	}
	e, _ := translateHarnessEvent(runtime.AgentEvent{Type: runtime.EventToolCallStart})
	if e.Kind != "tool_call" || e.Details.ToolPresent {
		t.Fatal(e)
	}
	e, _ = translateHarnessEvent(runtime.AgentEvent{Type: runtime.EventToolResult, Result: &tool.ToolResult{Metadata: map[string]any{"bad": make(chan int)}}})
	if !strings.Contains(e.Details.Metadata, "metadata unavailable") {
		t.Fatal(e)
	}
}

func TestGoalStreamPublishesBoundedToolDetailsAndUsageInOrder(t *testing.T) {
	backend := &backendFixture{run: func(context.Context, string) (<-chan BackendEvent, error) {
		ch := make(chan BackendEvent, 4)
		ch <- BackendEvent{Kind: "tool_call", Details: Details{ToolName: "read_file", ToolInput: `{"path":"file"}`}}
		ch <- BackendEvent{Kind: "tool_result", Details: Details{Output: strings.Repeat("界", MaxEventTextBytes)}}
		ch <- BackendEvent{Text: "answer"}
		ch <- BackendEvent{Kind: "usage", Done: true, Details: Details{UsageKnown: true, InputTokens: 10, OutputTokens: 2}}
		close(ch)
		return ch, nil
	}}
	stream, err := New(backend, Options{SessionID: "s", MaxIterations: 1}).Start(context.Background(), "prompt", nil)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	var sequence uint64
	for e := range stream.Events {
		if e.Sequence <= sequence || e.SessionID != "s" || e.RunID != 1 {
			t.Fatal(e)
		}
		sequence = e.Sequence
		if e.Kind == "state" {
			continue
		}
		kinds = append(kinds, e.Kind)
		if e.Kind == "tool_result" && (!e.Truncated || !e.Details.Truncated || len(e.Details.Output) > MaxEventTextBytes || !utf8.ValidString(e.Details.Output)) {
			t.Fatal("unbounded tool output")
		}
		if e.Kind == "usage" && (!e.Details.UsageKnown || e.Details.InputTokens != 10) {
			t.Fatal(e)
		}
	}
	if strings.Join(kinds, ",") != "tool_call,tool_result,text,usage,turn_end" {
		t.Fatal(kinds)
	}
	outcome, err := stream.Wait()
	if err != nil || outcome.Status != agentio.Completed {
		t.Fatal(outcome, err)
	}
	terminal := <-stream.Terminal
	if terminal.Kind != "terminal" || terminal.Sequence <= sequence {
		t.Fatal(terminal)
	}
}
