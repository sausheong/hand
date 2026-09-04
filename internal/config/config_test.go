package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sausheong/agcode/internal/config"
)

func TestLoad_CreatesDefaultOnFirstRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.json")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Model != config.DefaultModel {
		t.Fatalf("Model = %q, want default %q", cfg.Model, config.DefaultModel)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected Load to create %s, but ReadFile failed: %v", path, err)
	}
	if len(data) == 0 {
		t.Fatal("config file was created but is empty")
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := config.Config{Model: "openai/gpt-5"}

	if err := config.Save(path, want); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestSaveLoad_RoundTripWithBaseURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := config.Config{Model: "openai/gpt-5", BaseURL: "https://litellm.example.com"}

	if err := config.Save(path, want); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoad_MissingBaseURLDefaultsEmpty(t *testing.T) {
	// A config.json written before BaseURL existed has no "base_url"
	// key at all — Load must still parse it, with BaseURL="".
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"model":"anthropic/claude-sonnet-5"}`), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got.BaseURL != "" {
		t.Fatalf("BaseURL = %q, want empty for a pre-existing config file without base_url", got.BaseURL)
	}
}

func TestResolveModel_FlagOverridesConfig(t *testing.T) {
	cases := []struct {
		name      string
		flagValue string
		cfg       config.Config
		want      string
	}{
		{"flag empty uses config", "", config.Config{Model: "anthropic/claude-sonnet-5"}, "anthropic/claude-sonnet-5"},
		{"flag set overrides config", "qwen/qwen-max", config.Config{Model: "anthropic/claude-sonnet-5"}, "qwen/qwen-max"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := config.ResolveModel(tc.flagValue, tc.cfg)
			if got != tc.want {
				t.Fatalf("ResolveModel(%q, %+v) = %q, want %q", tc.flagValue, tc.cfg, got, tc.want)
			}
		})
	}
}

func TestResolveBaseURL_FlagOverridesConfig(t *testing.T) {
	cases := []struct {
		name      string
		flagValue string
		cfg       config.Config
		want      string
	}{
		{"flag empty uses config", "", config.Config{BaseURL: "https://litellm.example.com"}, "https://litellm.example.com"},
		{"flag set overrides config", "https://other-proxy.example.com", config.Config{BaseURL: "https://litellm.example.com"}, "https://other-proxy.example.com"},
		{"both empty stays empty", "", config.Config{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := config.ResolveBaseURL(tc.flagValue, tc.cfg)
			if got != tc.want {
				t.Fatalf("ResolveBaseURL(%q, %+v) = %q, want %q", tc.flagValue, tc.cfg, got, tc.want)
			}
		})
	}
}
