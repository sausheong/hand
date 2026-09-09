package app

import (
	"reflect"
	"testing"

	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
)

func TestInitialRuntimeConfigurationRejectsInvalidProfileAtomically(t *testing.T) {
	for _, profile := range []config.ModelProfile{
		{ContextLimit: -1}, {MaxOutput: -1},
		{ContextLimit: 100, MaxOutput: 100},
		{ContextLimit: 100, MaxOutput: 101},
		{ContextLimit: 8192, MaxOutput: 123, Reasoning: "unknown"},
	} {
		rt := &runtime.Runtime{ContextWindow: 32768, MaxOutputTokens: 512, Reasoning: llm.ReasoningHigh, DynamicIdentityHint: "original"}
		before := &runtime.Runtime{ContextWindow: 32768, MaxOutputTokens: 512, Reasoning: llm.ReasoningHigh, DynamicIdentityHint: "original"}
		if err := ConfigureInitialRuntime(rt, profile, "replacement"); err == nil {
			t.Fatalf("invalid profile accepted: %+v", profile)
		}
		if !reflect.DeepEqual(rt, before) {
			t.Fatal("invalid profile partially changed runtime")
		}
	}
	if err := ConfigureInitialRuntime(nil, config.ModelProfile{}, "hint"); err == nil {
		t.Fatal("missing runtime accepted")
	}
}

func TestInitialRuntimeConfigurationAppliesResolvedProfileAndDefaults(t *testing.T) {
	rt := &runtime.Runtime{}
	if err := ConfigureInitialRuntime(rt, config.ModelProfile{ContextLimit: 8192, MaxOutput: 123, Reasoning: "high"}, "selected"); err != nil {
		t.Fatal(err)
	}
	if rt.ContextWindow != 8192 || rt.MaxOutputTokens != 123 || rt.Reasoning != llm.ReasoningHigh || rt.DynamicIdentityHint != "selected" {
		t.Fatal("resolved settings not applied")
	}
	if err := ConfigureInitialRuntime(rt, config.ModelProfile{}, "default"); err != nil {
		t.Fatal(err)
	}
	if rt.ContextWindow != 0 || rt.MaxOutputTokens != 0 || rt.Reasoning != llm.ReasoningOff || rt.DynamicIdentityHint != "default" {
		t.Fatal("default settings retained previous configuration")
	}
}
