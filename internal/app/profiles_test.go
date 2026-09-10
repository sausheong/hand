package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/compaction"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
)

func TestProfileSwitchStagesRoutingAndCompactionAtomically(t *testing.T) {
	old := &controllerProvider{}
	rt := &runtime.Runtime{Provider: "local", Model: "old", LLM: old, ContextWindow: 4096, Reasoning: llm.ReasoningLow, FallbackModel: "retry", Compaction: &compaction.Manager{Summarizer: &compaction.Summarizer{Provider: old, Model: "old"}}}
	c := &Controller{Rt: rt, BaseURL: "http://localhost:9000/v1"}
	profiles := map[string]config.ModelProfile{
		"hosted": {Provider: "openai", Model: "hosted", CredentialEnv: "HOSTED_KEY", ContextLimit: 32768, MaxOutput: 123},
		"proxy":  {Provider: "openai", Model: "alias", Endpoint: "https://proxy.example/v1", CredentialEnv: "PROXY_KEY", ContextLimit: 16384, Reasoning: "high", ReasoningLevels: []string{"high"}},
	}
	if err := c.ConfigureProfiles(profiles, ""); err != nil {
		t.Fatal(err)
	}
	profiles["proxy"] = config.ModelProfile{Provider: "local", Model: "tampered"}
	var built []config.ModelProfile
	fail := true
	c.BuildProfileProvider = func(p config.ModelProfile) (llm.LLMProvider, error) {
		built = append(built, p)
		if fail {
			return nil, errors.New("credential unavailable")
		}
		return &controllerProvider{}, nil
	}
	if err := c.SwitchProfile("hosted"); err == nil {
		t.Fatal("failed construction accepted")
	}
	if c.CurrentModel() != "local/old" || c.ContextLimit() != 4096 || rt.LLM != old || rt.Compaction.Summarizer.Provider != old || c.BaseURL != "http://localhost:9000/v1" || rt.Reasoning != llm.ReasoningLow || rt.FallbackModel != "retry" {
		t.Fatal("failed switch changed configuration")
	}
	fail = false
	if err := c.SwitchProfile("hosted"); err != nil {
		t.Fatal(err)
	}
	if rt.MaxOutputTokens != 123 || c.CurrentProfile() != "hosted" || rt.ContextWindow != 32768 || rt.FallbackModel != "" || rt.Reasoning != llm.ReasoningOff || rt.Compaction.Summarizer.Provider != rt.LLM {
		t.Fatal("hosted switch inconsistent")
	}
	rt.FallbackModel = "hosted-fallback"
	if err := c.SwitchProfile("proxy"); err != nil {
		t.Fatal(err)
	}
	if rt.MaxOutputTokens != 2048 || c.CurrentProfile() != "proxy" || c.CurrentModel() != "openai/alias" || c.ContextLimit() != 16384 || rt.Reasoning != llm.ReasoningHigh || rt.FallbackModel != "hosted-fallback" || rt.Compaction.Summarizer.Provider != rt.LLM || rt.Compaction.Summarizer.Model != "alias" {
		t.Fatal("proxy switch inconsistent")
	}
	if built[1].Endpoint != "" || built[1].CredentialEnv != "HOSTED_KEY" || built[2].Endpoint != "https://proxy.example/v1" || built[2].CredentialEnv != "PROXY_KEY" {
		t.Fatal("profile boundary crossed", built)
	}
	if !reflect.DeepEqual(c.ProfileNames(), []string{"hosted", "proxy"}) {
		t.Fatal(c.ProfileNames())
	}
}

func TestProfileSwitchRejectsBusyAndInvalidBeforeConstruction(t *testing.T) {
	c := &Controller{Rt: &runtime.Runtime{Provider: "local", Model: "old"}}
	if err := c.ConfigureProfiles(map[string]config.ModelProfile{"bad-reasoning": {Provider: "local", Model: "m", Reasoning: "high"}}, ""); err != nil {
		t.Fatal(err)
	}
	calls := 0
	c.BuildProfileProvider = func(config.ModelProfile) (llm.LLMProvider, error) { calls++; return nil, nil }
	for _, name := range []string{"missing", "bad-reasoning"} {
		if c.SwitchProfile(name) == nil {
			t.Fatal("invalid profile accepted", name)
		}
	}
	_, release, err := c.owner().reserve(context.Background(), CheckingCompletion)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(c.SwitchProfile("bad-reasoning"), ErrBusy) {
		t.Fatal("active check did not prevent switch")
	}
	release()
	if calls != 0 || c.CurrentModel() != "local/old" {
		t.Fatal("invalid switch reached factory")
	}
}
