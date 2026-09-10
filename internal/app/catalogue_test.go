package app

import (
	"context"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/config"
)

func TestCataloguePreparationAndEndpointIsolation(t *testing.T) {
	for _, tc := range []struct {
		p               config.ModelProfile
		context, output int
		source          string
	}{
		{config.ModelProfile{Provider: "openai", Model: "gpt-4o-2024-08-06"}, 128000, 16384, "catalogue:" + config.ModelCatalogueVersion},
		{config.ModelProfile{Provider: "openai", Model: "gpt-4o-2024-08-06", ContextLimit: 32000, MaxOutput: 1000}, 32000, 1000, "explicit_override"},
		{config.ModelProfile{Provider: "openai", Model: "gpt-4o-2024-08-06", Endpoint: "https://proxy.example/v1"}, 8192, 2048, "conservative_fallback"},
		{config.ModelProfile{Provider: "local", Model: "gpt-4o-2024-08-06"}, 8192, 2048, "conservative_fallback"},
		{config.ModelProfile{Provider: "openrouter", Model: "openai/gpt-4o-2024-08-06"}, 8192, 2048, "conservative_fallback"},
		{config.ModelProfile{Provider: "openai", Model: "gpt-4o-unknown"}, 8192, 2048, "conservative_fallback"},
		{config.ModelProfile{Provider: "local", Model: "small", ContextLimit: 4096}, 4096, 2048, "explicit_override"},
	} {
		got, err := PrepareProfile(context.Background(), tc.p)
		if err != nil || got.ContextLimit != tc.context || got.MaxOutput != tc.output || !strings.HasPrefix(got.ContextSource, tc.source) {
			t.Fatalf("profile=%+v got=%+v err=%v", tc.p, got, err)
		}
	}
	p := config.ModelProfile{Provider: "openai", Model: "gpt-4o"}
	got, err := PrepareProfile(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.InputTypes) != 2 {
		t.Fatal("catalogue modalities missing")
	}
	got.InputTypes[0] = "tampered"
	again, err := PrepareProfile(context.Background(), p)
	if err != nil || again.InputTypes[0] != "text" {
		t.Fatal("catalogue state aliased")
	}
	p.InputTypes = []string{"text"}
	got, err = PrepareProfile(context.Background(), p)
	if err != nil || len(got.InputTypes) != 1 {
		t.Fatal("explicit input types overwritten")
	}
}
