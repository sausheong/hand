package app

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
)

func TestModelInfoEventsPreserveRequestedIdentityAcrossSwitch(t *testing.T) {
	rt := &runtime.Runtime{Provider: "openrouter", Model: "vendor/alias", ContextWindow: 24576}
	info := runtimeModelInfo(rt, config.ModelProfile{ProfileName: "proxy", ContextSource: "llamacpp_props:fixture#sha256=recorded"})
	s := New(&backendFixture{run: func(context.Context, string) (<-chan BackendEvent, error) { return completedStream(), nil }}, Options{SessionID: "s", MaxIterations: 1, Model: info})
	var first []Event
	if _, err := s.Execute(context.Background(), "one", nil, func(e Event) { first = append(first, e) }); err != nil {
		t.Fatal(err)
	}
	c := &Controller{Owner: s, Rt: rt, BuildProfileProvider: func(config.ModelProfile) (llm.LLMProvider, error) { return &controllerProvider{}, nil }}
	if err := c.ConfigureProfiles(map[string]config.ModelProfile{"next": {Provider: "local", Model: "next", ContextLimit: 16384}}, ""); err != nil {
		t.Fatal(err)
	}
	if err := c.SwitchProfile("next"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Execute(context.Background(), "two", nil, func(e Event) {
		if e.Model.Profile != "next" || e.Model.RequestedModel != "local/next" || e.Model.ContextLimit != 16384 || e.Model.ContextSource != "explicit_override" {
			t.Error(e.Model)
		}
	}); err != nil {
		t.Fatal(err)
	}
	for _, e := range first {
		if e.Model != info || e.Model.ServingModelKnown || e.Model.ServingModel != "" {
			t.Fatal("prior event changed or alias became serving proof", e.Model)
		}
	}
}

func TestModelInfoBoundPreservesUnicode(t *testing.T) {
	info := boundModelInfo(ModelInfo{Profile: "profile", RequestedModel: strings.Repeat("界", MaxEventTextBytes), ContextSource: "source"})
	if !info.Truncated || len(info.Profile)+len(info.RequestedModel)+len(info.ContextSource) > MaxEventTextBytes || !utf8.ValidString(info.RequestedModel) {
		t.Fatal("unbounded or invalid metadata")
	}
}

func TestHarnessModelInfoUsesPreparedProfileProvenance(t *testing.T) {
	rt := &runtime.Runtime{Provider: "local", Model: "alias", ContextWindow: 16384}
	service := NewHarness(rt, nil, nil, "", 1, config.ModelProfile{ProfileName: "local", ContextSource: "server-response#sha256=abc"})
	if service.options.Model.Profile != "local" || service.options.Model.ContextSource != "server-response#sha256=abc" || service.options.Model.ContextLimit != 16384 || service.options.Model.ServingModelKnown {
		t.Fatal(service.options.Model)
	}
}
