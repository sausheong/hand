package config_test

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/sausheong/hand/internal/config"
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
	if !reflect.DeepEqual(got, want) {
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
	if !reflect.DeepEqual(got, want) {
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

func TestResolveFallbackModel_FlagOverridesConfig(t *testing.T) {
	cases := []struct {
		name      string
		flagValue string
		cfg       config.Config
		want      string
	}{
		{"flag empty uses config", "", config.Config{FallbackModel: "anthropic/claude-haiku-4-5"}, "anthropic/claude-haiku-4-5"},
		{"flag set overrides config", "openai/gpt-5-mini", config.Config{FallbackModel: "anthropic/claude-haiku-4-5"}, "openai/gpt-5-mini"},
		{"both empty stays empty", "", config.Config{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := config.ResolveFallbackModel(tc.flagValue, tc.cfg)
			if got != tc.want {
				t.Fatalf("ResolveFallbackModel(%q, %+v) = %q, want %q", tc.flagValue, tc.cfg, got, tc.want)
			}
		})
	}
}

func TestSaveLoad_RoundTripWithMCPServers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := config.Config{
		Model: "anthropic/claude-sonnet-5",
		MCPServers: []config.MCPServer{
			{Name: "github", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"}, Env: map[string]string{"TOKEN": "x"}, Trusted: true},
			{Name: "remote", URL: "https://example.com/mcp", Headers: map[string]string{"Authorization": "Bearer x"}},
		},
	}

	if err := config.Save(path, want); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !reflect.DeepEqual(got.MCPServers, want.MCPServers) {
		t.Fatalf("MCPServers = %+v, want %+v", got.MCPServers, want.MCPServers)
	}
}

func TestLoad_MissingMCPServersDefaultsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"model":"anthropic/claude-sonnet-5"}`), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(got.MCPServers) != 0 {
		t.Fatalf("MCPServers = %+v, want empty for a pre-existing config file without mcp_servers", got.MCPServers)
	}
}

func TestSaveLoad_RoundTripWithHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := config.Config{
		Model: "anthropic/claude-sonnet-5",
		Hooks: []config.HookConfig{
			{Event: "PreToolUse", Matcher: "bash", Command: "sh", Args: []string{"-c", "exit 0"}, Timeout: 10},
			{Event: "Stop", Command: "notify-send", Args: []string{"hand finished"}},
		},
	}

	if err := config.Save(path, want); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !reflect.DeepEqual(got.Hooks, want.Hooks) {
		t.Fatalf("Hooks = %+v, want %+v", got.Hooks, want.Hooks)
	}
}

func TestLoad_MissingHooksDefaultsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"model":"anthropic/claude-sonnet-5"}`), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(got.Hooks) != 0 {
		t.Fatalf("Hooks = %+v, want empty for a pre-existing config file without hooks", got.Hooks)
	}
}

func TestToServerConfigs_FieldByFieldMapping(t *testing.T) {
	cfg := config.Config{
		MCPServers: []config.MCPServer{
			{
				Name:    "github",
				Command: "npx",
				Args:    []string{"-y", "server"},
				Env:     map[string]string{"TOKEN": "x"},
				Trusted: true,
			},
			{
				Name:    "remote",
				URL:     "https://example.com/mcp",
				Headers: map[string]string{"Authorization": "Bearer x"},
			},
		},
	}

	got := cfg.ToServerConfigs()
	if len(got) != 2 {
		t.Fatalf("ToServerConfigs() len = %d, want 2", len(got))
	}

	if got[0].Name != "github" || got[0].Command != "npx" || got[0].Args[0] != "-y" || got[0].Args[1] != "server" || got[0].Env["TOKEN"] != "x" {
		t.Fatalf("ToServerConfigs()[0] = %+v, want fields copied from MCPServers[0]", got[0])
	}
	if got[0].URL != "" || got[0].Headers != nil {
		t.Fatalf("ToServerConfigs()[0] = %+v, want URL/Headers left zero for a stdio server", got[0])
	}

	if got[1].Name != "remote" || got[1].URL != "https://example.com/mcp" || got[1].Headers["Authorization"] != "Bearer x" {
		t.Fatalf("ToServerConfigs()[1] = %+v, want fields copied from MCPServers[1]", got[1])
	}
	if got[1].Command != "" || got[1].Args != nil {
		t.Fatalf("ToServerConfigs()[1] = %+v, want Command/Args left zero for an HTTP server", got[1])
	}
}

func TestToServerConfigs_EmptyReturnsNil(t *testing.T) {
	got := config.Config{}.ToServerConfigs()
	if got != nil {
		t.Fatalf("ToServerConfigs() = %+v, want nil for a config with no MCP servers", got)
	}
}

func TestTrustedMCPServers_ReturnsOnlyTrustedNames(t *testing.T) {
	cfg := config.Config{
		MCPServers: []config.MCPServer{
			{Name: "github", Command: "npx", Trusted: true},
			{Name: "untrusted-server", Command: "npx", Trusted: false},
			{Name: "remote", URL: "https://example.com/mcp", Trusted: true},
		},
	}

	got := cfg.TrustedMCPServers()

	if !got["github"] {
		t.Error(`TrustedMCPServers()["github"] = false, want true`)
	}
	if !got["remote"] {
		t.Error(`TrustedMCPServers()["remote"] = false, want true`)
	}
	if got["untrusted-server"] {
		t.Error(`TrustedMCPServers()["untrusted-server"] = true, want false`)
	}
	if len(got) != 2 {
		t.Fatalf("TrustedMCPServers() = %+v, want exactly 2 entries", got)
	}
}

func TestAllMCPServerNames_ReturnsEveryNameTrustedOrNot(t *testing.T) {
	cfg := config.Config{
		MCPServers: []config.MCPServer{
			{Name: "github", Command: "npx", Trusted: true},
			{Name: "untrusted-server", Command: "npx", Trusted: false},
			{Name: "remote", URL: "https://example.com/mcp", Trusted: true},
		},
	}

	got := cfg.AllMCPServerNames()

	want := []string{"github", "untrusted-server", "remote"}
	if len(got) != len(want) {
		t.Fatalf("AllMCPServerNames() = %v, want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("AllMCPServerNames()[%d] = %q, want %q", i, got[i], name)
		}
	}
}

func TestAllMCPServerNames_EmptyReturnsNil(t *testing.T) {
	got := config.Config{}.AllMCPServerNames()
	if got != nil {
		t.Fatalf("AllMCPServerNames() = %+v, want nil for a config with no MCP servers", got)
	}
}

func TestResolveMaxTurns_FlagOverridesConfigOverridesDefault(t *testing.T) {
	cases := []struct {
		name      string
		flagValue int
		cfg       config.Config
		want      int
	}{
		{"flag and config both zero uses default", 0, config.Config{}, config.DefaultMaxTurns},
		{"flag zero uses config", 0, config.Config{MaxTurns: 10}, 10},
		{"flag set overrides config", 5, config.Config{MaxTurns: 10}, 5},
		{"flag set overrides default", 5, config.Config{}, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := config.ResolveMaxTurns(tc.flagValue, tc.cfg)
			if got != tc.want {
				t.Fatalf("ResolveMaxTurns(%d, %+v) = %d, want %d", tc.flagValue, tc.cfg, got, tc.want)
			}
		})
	}
}

func TestResolveMaxGoalIterations_FlagOverridesConfigOverridesDefault(t *testing.T) {
	cases := []struct {
		name      string
		flagValue int
		cfg       config.Config
		want      int
	}{
		{"flag and config both zero uses default", 0, config.Config{}, config.DefaultMaxGoalIterations},
		{"flag zero uses config", 0, config.Config{MaxGoalIterations: 25}, 25},
		{"flag set overrides config", 5, config.Config{MaxGoalIterations: 25}, 5},
		{"flag set overrides default", 5, config.Config{}, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := config.ResolveMaxGoalIterations(tc.flagValue, tc.cfg)
			if got != tc.want {
				t.Fatalf("ResolveMaxGoalIterations(%d, %+v) = %d, want %d", tc.flagValue, tc.cfg, got, tc.want)
			}
		})
	}
}

func TestSaveLoad_RoundTripWithMaxGoalIterations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := config.Config{Model: "anthropic/claude-sonnet-5", MaxGoalIterations: 25}

	if err := config.Save(path, want); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got.MaxGoalIterations != 25 {
		t.Fatalf("MaxGoalIterations = %d, want 25", got.MaxGoalIterations)
	}
}

// Regression: MCPServers entries may carry bearer tokens or other
// secrets in Headers/Env, so config.json is treated like a credential
// file — it must not be readable by other local users on a shared
// machine.
func TestSave_WritesOwnerOnlyPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't apply on windows")
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Config{Model: "anthropic/claude-sonnet-5"}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config file permissions = %o, want 0600", perm)
	}
}

// Regression: an existing config file from before Save started using
// 0o600 should be tightened the next time it's loaded, not left
// world/group-readable indefinitely.
func TestLoad_TightensLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't apply on windows")
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Config{Model: "anthropic/claude-sonnet-5"}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("Chmod returned error: %v", err)
	}

	if _, err := config.Load(path); err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config file permissions after Load = %o, want 0600", perm)
	}
}

func TestResolveMarkdownStyle_FlagOverridesConfigOverridesDefault(t *testing.T) {
	cases := []struct {
		name      string
		flagValue string
		cfg       config.Config
		want      string
	}{
		{"flag and config both empty uses default", "", config.Config{}, config.DefaultMarkdownStyle},
		{"flag empty uses valid config value", "", config.Config{MarkdownStyle: "light"}, "light"},
		{"flag set overrides config", "pink", config.Config{MarkdownStyle: "light"}, "pink"},
		{"flag set overrides default", "dracula", config.Config{}, "dracula"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := config.ResolveMarkdownStyle(tc.flagValue, tc.cfg)
			if got != tc.want {
				t.Fatalf("ResolveMarkdownStyle(%q, %+v) = %q, want %q", tc.flagValue, tc.cfg, got, tc.want)
			}
		})
	}
}

// Regression: "auto" is a real glamour style name (it triggers
// termenv's interactive terminal-background query — see
// internal/tui/markdown.go for why that's unsafe alongside Bubble Tea's
// own stdin reader), so it must be rejected the same as any other
// unrecognized string, not accidentally let through because it happens
// to be a name glamour understands.
func TestResolveMarkdownStyle_RejectsAutoAndUnknownValues(t *testing.T) {
	for _, style := range []string{"auto", "bogus", "AUTO", "Dark"} {
		t.Run(style, func(t *testing.T) {
			got := config.ResolveMarkdownStyle(style, config.Config{})
			if got != config.DefaultMarkdownStyle {
				t.Fatalf("ResolveMarkdownStyle(%q, {}) = %q, want the default %q", style, got, config.DefaultMarkdownStyle)
			}
			got = config.ResolveMarkdownStyle("", config.Config{MarkdownStyle: style})
			if got != config.DefaultMarkdownStyle {
				t.Fatalf("ResolveMarkdownStyle(\"\", {MarkdownStyle: %q}) = %q, want the default %q", style, got, config.DefaultMarkdownStyle)
			}
		})
	}
}

func TestValidMarkdownStyles_ExcludesAuto(t *testing.T) {
	for _, s := range config.ValidMarkdownStyles {
		if s == "auto" {
			t.Fatal("ValidMarkdownStyles must never include \"auto\" — it triggers glamour's terminal-querying auto-detection, exactly what this allow-list exists to keep out")
		}
	}
}

func TestResolveCompactionThreshold_FlagOverridesConfigOverridesDefault(t *testing.T) {
	cases := []struct {
		name      string
		flagValue float64
		cfg       config.Config
		want      float64
	}{
		{"flag and config both zero uses default", 0, config.Config{}, config.DefaultCompactionThreshold},
		{"flag zero uses valid config value", 0, config.Config{CompactionThreshold: 0.5}, 0.5},
		{"flag set overrides config", 0.25, config.Config{CompactionThreshold: 0.5}, 0.25},
		{"flag set overrides default", 0.3, config.Config{}, 0.3},
		{"flag of exactly 1 is valid (compact only once truly full)", 1, config.Config{}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := config.ResolveCompactionThreshold(tc.flagValue, tc.cfg)
			if got != tc.want {
				t.Fatalf("ResolveCompactionThreshold(%v, %+v) = %v, want %v", tc.flagValue, tc.cfg, got, tc.want)
			}
		})
	}
}

// Regression: a threshold outside (0, 1] means something degenerate to
// harness's own check (estimate > threshold*window) — zero or negative
// compacts on every single turn, above 1 never compacts at all — so
// both must be rejected as if unset, from either the flag or the config
// file, not passed through to produce one of those extremes.
func TestResolveCompactionThreshold_RejectsOutOfRangeValues(t *testing.T) {
	for _, bad := range []float64{0, -0.5, 1.1, 5} {
		got := config.ResolveCompactionThreshold(bad, config.Config{})
		if got != config.DefaultCompactionThreshold {
			t.Fatalf("ResolveCompactionThreshold(%v, {}) = %v, want the default %v", bad, got, config.DefaultCompactionThreshold)
		}
		got = config.ResolveCompactionThreshold(0, config.Config{CompactionThreshold: bad})
		if got != config.DefaultCompactionThreshold {
			t.Fatalf("ResolveCompactionThreshold(0, {CompactionThreshold: %v}) = %v, want the default %v", bad, got, config.DefaultCompactionThreshold)
		}
	}
}

// Regression: NaN and +/-Inf both fail every ordinary comparison against
// (0, 1] (NaN compares false to everything; +Inf and -Inf compare false
// or true in ways that could slip past a careless check), so a flag or
// config value corrupted into one of these must still fall back to the
// default rather than propagate into harness's compaction math.
func TestResolveCompactionThreshold_RejectsNaNAndInf(t *testing.T) {
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		got := config.ResolveCompactionThreshold(bad, config.Config{})
		if got != config.DefaultCompactionThreshold {
			t.Fatalf("ResolveCompactionThreshold(%v, {}) = %v, want the default %v", bad, got, config.DefaultCompactionThreshold)
		}
		got = config.ResolveCompactionThreshold(0, config.Config{CompactionThreshold: bad})
		if got != config.DefaultCompactionThreshold {
			t.Fatalf("ResolveCompactionThreshold(0, {CompactionThreshold: %v}) = %v, want the default %v", bad, got, config.DefaultCompactionThreshold)
		}
	}
}
