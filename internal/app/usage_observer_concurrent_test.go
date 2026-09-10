package app

import (
	"context"
	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/session"
	"sync"
	"testing"
)

func TestConcurrentUsageObserversRejectOverflowWithoutPoisoningJournal(t *testing.T) {
	sess := session.NewSession("hand", "key")
	ctx, result := observeSessionUsage(context.Background(), sess)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			stream, err := llm.ObserveChat(ctx, llm.ChatRequest{Model: "model"}, llm.CallGeneration, func(context.Context, llm.ChatRequest) (<-chan llm.ChatEvent, error) {
				ch := make(chan llm.ChatEvent, 1)
				ch <- llm.ChatEvent{Type: llm.EventDone, Usage: &llm.Usage{InputTokens: int(^uint(0) >> 1)}}
				close(ch)
				return ch, nil
			})
			if err != nil {
				t.Error(err)
				return
			}
			for range stream {
			}
		}()
	}
	close(start)
	wg.Wait()
	if result() == nil {
		t.Fatal("overflow not surfaced")
	}
	summary, err := sessionio.ReadUsage(sess)
	if err != nil || summary.Requests != 1 || summary.Total.InputTokens != int(^uint(0)>>1) {
		t.Fatal("concurrent writes poisoned accounting", summary, err)
	}
}
