package main

import (
	"context"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
)

type completedProvider struct{ llm.LLMProvider }

func (completedProvider) ChatStream(context.Context, llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	ch := make(chan llm.ChatEvent, 2)
	ch <- llm.ChatEvent{Type: llm.EventTextDelta, Text: "answer"}
	ch <- llm.ChatEvent{Type: llm.EventDone}
	close(ch)
	return ch, nil
}

func TestOneShotMandatoryValidationFailureReturnsError(t *testing.T) {
	workspace := t.TempDir()
	reason := ""
	hooks := []config.HookConfig{{Event: "Stop", Command: "sh", Args: []string{"-c", "exit 1"}}}
	rt := &runtime.Runtime{LLM: completedProvider{}, Tools: tool.NewRegistry(), Session: session.NewSession("hand", "test"), AgentID: "hand", Model: "test", Workspace: workspace, MaxTurns: 1}
	rt.AgentLoop.Hooks = agentio.BuildLifecycleHooks(hooks, workspace, nil, &reason)
	err := runOneShot(context.Background(), rt, "hello", hooks, workspace, &reason, 2)
	if err == nil || !strings.Contains(err.Error(), "mandatory Stop validation failed") {
		t.Fatalf("one-shot returned %v", err)
	}
}

func (completedProvider) NormalizeToolSchema(defs []llm.ToolDef) ([]llm.ToolDef, []llm.Diagnostic) {
	return defs, nil
}
