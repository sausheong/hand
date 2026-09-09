package app

import (
	"context"
	"github.com/sausheong/harness/compaction"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"strings"
	"testing"
)

type focusProvider struct {
	llm.LLMProvider
	requests []llm.ChatRequest
}

func (p *focusProvider) ChatStream(_ context.Context, req llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	p.requests = append(p.requests, req)
	ch := make(chan llm.ChatEvent, 2)
	ch <- llm.ChatEvent{Type: llm.EventTextDelta, Text: "<summary>Summary with decisions and unresolved work</summary>"}
	ch <- llm.ChatEvent{Type: llm.EventDone}
	close(ch)
	return ch, nil
}
func TestManualCompactionForwardsPinsFocusAndBudget(t *testing.T) {
	sess := session.NewSession("hand", "focus")
	for i := 0; i < 10; i++ {
		sess.Append(session.UserMessageEntry("task detail"))
		sess.Append(session.AssistantMessageEntry("work detail"))
	}
	provider := &focusProvider{}
	rt := &runtime.Runtime{Session: sess, Compaction: &compaction.Manager{Summarizer: &compaction.Summarizer{Provider: provider, Model: "summary-fixture", MaxOutputTokens: 1024}, PreserveTurns: 2}}
	c := &Controller{Rt: rt}
	ctx := context.Background()
	if err := c.SetContextPin(ctx, runtime.ContextPin{ID: "constraint", Kind: "constraint", Text: "KEEP SOURCE IDENTIFIERS"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CompactWithFocus(ctx, strings.Repeat("x", 4097)); err == nil || len(provider.requests) != 0 {
		t.Fatal("invalid focus dispatched")
	}
	result, err := c.CompactWithFocus(ctx, "Prioritise unresolved verification")
	if err != nil || !result.Compacted || len(provider.requests) != 1 {
		t.Fatalf("compaction %+v %v", result, err)
	}
	request := provider.requests[0]
	if request.MaxTokens != 1024 || request.Model != "summary-fixture" {
		t.Fatalf("configuration lost: %+v", request)
	}
	for _, text := range []string{"KEEP SOURCE IDENTIFIERS", "Prioritise unresolved verification"} {
		if !strings.Contains(request.Messages[0].Content, text) {
			t.Fatal("missing focus or pin", text)
		}
	}
	pins, err := c.ContextPins(ctx)
	if err != nil || len(pins) != 1 {
		t.Fatal("compaction lost pins")
	}
}
