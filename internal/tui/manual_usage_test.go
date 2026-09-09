package tui

import (
	"context"
	"testing"

	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/harness/compaction"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/llm/llmtest"
	"github.com/sausheong/harness/session"
)

type usageSummaryProvider struct{ llmtest.Base }

func (p *usageSummaryProvider) ChatStream(context.Context, llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	events := make(chan llm.ChatEvent, 2)
	events <- llm.ChatEvent{Type: llm.EventTextDelta, Text: "Summary: Continue the original task with its existing constraints."}
	events <- llm.ChatEvent{Type: llm.EventDone, Usage: &llm.Usage{InputTokens: 60, OutputTokens: 8}}
	close(events)
	return events, nil
}

func TestManualCompactionUsagePersistsAndRefreshes(t *testing.T) {
	m, manager, _ := sessionUIFixture(t)
	sess := m.controller.Rt.Session
	for i := 0; i < 8; i++ {
		sess.Append(session.UserMessageEntry("question"))
		sess.Append(session.AssistantMessageEntry("answer"))
	}
	m.controller.Rt.Compaction = &compaction.Manager{PreserveTurns: 1, Summarizer: &compaction.Summarizer{Provider: &usageSummaryProvider{}, Model: "summary-model"}}
	cmd := m.runCompactCommand()
	if cmd == nil {
		t.Fatal("manual compaction not started")
	}
	result := cmd().(compactResultMsg)
	if result.err != nil || !result.result.Compacted {
		t.Fatal("manual compaction did not commit", result.result, result.err)
	}
	m.Update(result)
	if m.compacting || m.sessionUsage.InputTokens != 60 || m.sessionUsage.OutputTokens != 8 || m.usageRequests != 1 || m.usageUnknown != 0 {
		t.Fatal("manual compaction usage missing", m.sessionUsage, m.usageRequests, m.usageUnknown)
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	selected, err := manager.Open(context.Background(), sess.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	m.controller.Rt.Session = selected.Session
	usage, err := sessionio.ReadUsage(selected.Session)
	if err != nil || usage.Requests != 1 || usage.Total.InputTokens != 60 {
		t.Fatal("manual usage lost on restart", usage, err)
	}
}

func TestCancelledManualCompactionRetainsUnknownAttempt(t *testing.T) {
	m, provider := compactionTestModel(t)
	cmd := m.runCompactCommand()
	completed := make(chan compactResultMsg, 1)
	go func() { completed <- cmd().(compactResultMsg) }()
	<-provider.entered
	m.compactCancelled = true
	m.compactCancel()
	result := <-completed
	m.Update(result)
	if m.compacting || m.usageRequests != 1 || m.usageUnknown != 1 {
		t.Fatal("cancelled summarisation usage discarded", m.usageRequests, m.usageUnknown)
	}
}
