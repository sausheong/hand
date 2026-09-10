package app

import (
	"context"
	"errors"
	"testing"

	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/llm/llmtest"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
)

type persistedUsageProvider struct {
	llmtest.Base
	unknown, fail bool
}

func (p *persistedUsageProvider) ChatStream(context.Context, llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	if p.fail {
		return nil, errors.New("provider rejected request")
	}
	ch := make(chan llm.ChatEvent, 1)
	var usage *llm.Usage
	if !p.unknown {
		usage = &llm.Usage{InputTokens: 40, OutputTokens: 5, CacheReadInputTokens: 20}
	}
	ch <- llm.ChatEvent{Type: llm.EventDone, Usage: usage}
	close(ch)
	return ch, nil
}

func TestHarnessBackendPersistsAttemptUsageBeforeCompletion(t *testing.T) {
	for _, kind := range []string{"reported", "unknown", "failed"} {
		t.Run(kind, func(t *testing.T) {
			store := session.NewStore(t.TempDir())
			if err := store.Create("hand", "key"); err != nil {
				t.Fatal(err)
			}
			sess, err := store.LoadExclusive("hand", "key")
			if err != nil {
				t.Fatal(err)
			}
			backend := &HarnessBackend{Runtime: &runtime.Runtime{LLM: &persistedUsageProvider{unknown: kind == "unknown", fail: kind == "failed"}, Session: sess, Tools: tool.NewRegistry(), AgentID: "hand", Model: "test", MaxTurns: 1}}
			stream, err := backend.Run(context.Background(), "go", nil)
			if err != nil {
				t.Fatal(err)
			}
			summarySeen := false
			for event := range stream {
				if event.Kind == "session_usage" {
					summarySeen = true
					if event.Details.UsageRequests != 1 || event.Details.UsagePriorUnknown {
						t.Fatal(event)
					}
				}
			}
			if !summarySeen {
				t.Fatal("missing terminal accounting snapshot")
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
			if err != nil || summary.Requests != 1 {
				t.Fatal(summary, err)
			}
			if kind == "reported" {
				if summary.Total.InputTokens != 40 || summary.Total.OutputTokens != 5 || summary.Unknown != 0 {
					t.Fatal(summary)
				}
			} else if summary.Unknown != 1 {
				t.Fatal("missing usage became zero", summary)
			}
		})
	}
}
