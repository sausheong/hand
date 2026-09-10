package tui

import (
	"context"
	"testing"
	"time"

	"github.com/sausheong/harness/llm"
)

type heldCompactionProvider struct {
	llm.LLMProvider
	entered, cancelled, release chan struct{}
}

func (p *heldCompactionProvider) ChatStream(ctx context.Context, _ llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	close(p.entered)
	<-ctx.Done()
	close(p.cancelled)
	<-p.release
	return nil, ctx.Err()
}

func TestCompactionShutdownBeforeCommandDispatch(t *testing.T) {
	m, provider := compactionTestModel(t)
	m.runCompactCommand()
	closed := make(chan struct{})
	go func() { m.CloseApplication(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("undispatched compaction prevented shutdown")
	}
	select {
	case <-provider.entered:
		t.Fatal("shutdown dispatched queued provider call")
	default:
	}
}

func TestCompactionShutdownJoinsBlockedProvider(t *testing.T) {
	m, _ := compactionTestModel(t)
	provider := &heldCompactionProvider{entered: make(chan struct{}), cancelled: make(chan struct{}), release: make(chan struct{})}
	m.controller.Rt.Compaction.Summarizer.Provider = provider
	cmd := m.runCompactCommand()
	commandReturned := make(chan struct{})
	go func() { cmd(); close(commandReturned) }()
	<-provider.entered
	closed := make(chan struct{})
	go func() { m.CloseApplication(); close(closed) }()
	<-provider.cancelled
	select {
	case <-closed:
		t.Fatal("shutdown abandoned exiting provider")
	case <-time.After(20 * time.Millisecond):
	}
	close(provider.release)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not join provider")
	}
	select {
	case <-commandReturned:
	case <-time.After(time.Second):
		t.Fatal("command result remained blocked")
	}
	if m.controller.Rt.Compaction.HasInFlight(m.controller.Rt.Session) {
		t.Fatal("compaction remains in flight after shutdown")
	}
}
