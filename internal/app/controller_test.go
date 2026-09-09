package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sausheong/harness/compaction"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
)

type controllerProvider struct {
	llm.LLMProvider
	entered, release chan struct{}
}

func (p *controllerProvider) ChatStream(ctx context.Context, _ llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	close(p.entered)
	<-ctx.Done()
	<-p.release
	return nil, ctx.Err()
}
func (p *controllerProvider) NormalizeToolSchema(defs []llm.ToolDef) ([]llm.ToolDef, []llm.Diagnostic) {
	return defs, nil
}

func TestCrossProviderSwitchDoesNotForwardEndpointAndIsAtomic(t *testing.T) {
	old, newProvider := &controllerProvider{}, &controllerProvider{}
	rt := &runtime.Runtime{Provider: "local", Model: "old", LLM: old, FallbackModel: "fallback", DynamicIdentityHint: "original", Compaction: &compaction.Manager{Summarizer: &compaction.Summarizer{Provider: old, Model: "old"}}}
	controller := &Controller{Rt: rt, BaseURL: "http://private-proxy.invalid/v1"}
	failure := true
	controller.BuildProvider = func(provider, endpoint string) (llm.LLMProvider, error) {
		if provider != "openai" || endpoint != "" {
			t.Fatalf("cross-provider route %q %q", provider, endpoint)
		}
		if failure {
			return nil, errors.New("missing credential")
		}
		return newProvider, nil
	}
	if err := controller.SwitchModel("openai/new"); err == nil {
		t.Fatal("failed factory accepted")
	}
	if rt.LLM != old || rt.Model != "old" || rt.Provider != "local" || rt.FallbackModel != "fallback" || rt.DynamicIdentityHint != "original" || controller.BaseURL != "http://private-proxy.invalid/v1" || rt.Compaction.Summarizer.Provider != old {
		t.Fatal("failed switch mutated active configuration")
	}
	failure = false
	if err := controller.SwitchModel("openai/new"); err != nil {
		t.Fatal(err)
	}
	if rt.LLM != newProvider || rt.Model != "new" || rt.Provider != "openai" || rt.FallbackModel != "" || controller.BaseURL != "" || rt.Compaction.Summarizer.Provider != newProvider || rt.Compaction.Summarizer.Model != "new" {
		t.Fatal("successful switch not committed consistently")
	}
}

func TestControllerRunBlocksConfigurationUntilCancellationJoins(t *testing.T) {
	provider := &controllerProvider{entered: make(chan struct{}), release: make(chan struct{})}
	rt := &runtime.Runtime{Provider: "local", Model: "test", LLM: provider, Session: session.NewSession("hand", "test"), Tools: tool.NewRegistry(), MaxTurns: 1}
	controller := &Controller{Rt: rt, Owner: NewHarness(rt, nil, nil, t.TempDir(), 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := controller.Owner.Start(ctx, "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := make(chan struct{})
	go func() {
		for range stream.Events {
		}
		stream.Wait()
		close(joined)
	}()
	select {
	case <-provider.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("provider did not start")
	}
	if err := controller.SwitchModel("local/new"); !errors.Is(err, ErrBusy) {
		t.Fatalf("switch during run: %v", err)
	}
	cancel()
	if err := controller.NewSession(); !errors.Is(err, ErrBusy) {
		t.Fatalf("session change while cancellation unwinds: %v", err)
	}
	close(provider.release)
	select {
	case <-joined:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled run did not join")
	}
	if err := controller.SwitchModel("local/new"); err != nil {
		t.Fatal(err)
	}
	if controller.CurrentModel() != "local/new" {
		t.Fatal(controller.CurrentModel())
	}
}

func TestControllerManualCompactionBlocksSwitchUntilJoin(t *testing.T) {
	provider := &controllerProvider{entered: make(chan struct{}), release: make(chan struct{})}
	sess := session.NewSession("hand", "test")
	for range 8 {
		sess.Append(session.UserMessageEntry("question"))
		sess.Append(session.AssistantMessageEntry("answer"))
	}
	controller := &Controller{Rt: &runtime.Runtime{Provider: "local", Model: "test", Session: sess, Compaction: &compaction.Manager{PreserveTurns: 1, Summarizer: &compaction.Summarizer{Provider: provider, Model: "test", Timeout: time.Second}}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	joined := make(chan struct{})
	go func() { defer close(joined); controller.Compact(ctx) }()
	select {
	case <-provider.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("compaction did not start")
	}
	cancel()
	if err := controller.SwitchModel("local/new"); !errors.Is(err, ErrBusy) {
		t.Fatalf("switch while compaction unwinds: %v", err)
	}
	close(provider.release)
	select {
	case <-joined:
	case <-time.After(3 * time.Second):
		t.Fatal("compaction did not join")
	}
	if err := controller.SwitchModel("local/new"); err != nil {
		t.Fatal(err)
	}
}
