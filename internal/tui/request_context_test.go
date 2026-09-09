package tui

import (
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/llm"
	"strings"
	"testing"
)

func TestTenRequestsKeepContextSeparateFromTurnAndCacheTotals(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	defer m.CloseApplication()
	m.SetContextLimit(100000)
	for i := 0; i < 10; i++ {
		m.renderApplicationEvent(app.Event{Kind: "context_usage", Details: app.Details{UsageKnown: true, InputTokens: 20000, OutputTokens: 100, CacheReadInputTokens: 15000}})
	}
	m.renderApplicationEvent(app.Event{Kind: "usage", Details: app.Details{UsageKnown: true, InputTokens: 200000, OutputTokens: 1000, CacheReadInputTokens: 150000}})
	if !strings.Contains(m.contextSummary(), "ctx 20k/100k (20%)") {
		t.Fatal("context uses cumulative turn or cache twice", m.contextSummary())
	}
	if totalTokens(m.sessionUsage) != 201000 || contextTokens(&llm.Usage{InputTokens: 20000, CacheReadInputTokens: 15000}) != 20000 {
		t.Fatal("cache charged twice")
	}
	m.renderApplicationEvent(app.Event{Kind: "context_usage"})
	if !strings.Contains(m.contextSummary(), "unknown") {
		t.Fatal("unreported request retained stale context", m.contextSummary())
	}
}
