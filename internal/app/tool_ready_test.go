package app

import (
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"testing"
)

func TestToolReadyPreservesCompleteCommand(t *testing.T) {
	raw := `{"command":"git status\ngit diff"}`
	event, ok := translateHarnessEvent(runtime.AgentEvent{Type: runtime.EventToolCallReady, ToolCall: &llm.ToolCall{ID: "one", Name: "bash", Input: []byte(raw)}})
	if !ok || event.Kind != "tool_call_ready" || event.Details.ToolInput != raw || event.Details.ToolID != "one" {
		t.Fatalf("%+v", event)
	}
}
