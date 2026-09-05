// Package config reads and writes Hand's user-level configuration
// file at ~/.hand/config.json.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sausheong/harness/tools/mcp"
)

// DefaultModel is used when no config file exists yet.
const DefaultModel = "anthropic/claude-sonnet-5"

// DefaultMaxTurns caps the agent's tool-use loop for a single run when
// neither --max-turns nor config.json's max_turns is set.
const DefaultMaxTurns = 50

// Config is the on-disk shape of ~/.hand/config.json. API keys are
// never stored here — each provider reads its key from its own
// standard environment variable.
type Config struct {
	Model string `json:"model"`
	// BaseURL overrides the selected provider's default API endpoint,
	// e.g. to point at a LiteLLM proxy or other OpenAI-compatible
	// gateway. Empty means use the provider's own default. Not every
	// provider supports this — harness's Gemini provider has no
	// base-URL parameter.
	BaseURL string `json:"base_url,omitempty"`
	// MaxTurns caps the agent's tool-use loop for a single run. Zero (or
	// absent) means DefaultMaxTurns.
	MaxTurns int `json:"max_turns,omitempty"`
	// FallbackModel is the "provider/model" to retry against on a
	// transient provider error (429/5xx), same form as Model and passed
	// through unparsed — runtime.AgentSpec.FallbackModel does its own
	// same-provider validation. Empty means no fallback.
	FallbackModel string `json:"fallback_model,omitempty"`
	// MCPServers lists the MCP servers to connect at startup. Empty
	// means no extensions — hand's built-in tool set only.
	MCPServers []MCPServer `json:"mcp_servers,omitempty"`
}

// MCPServer is one entry in config.json's mcp_servers list. Exactly one
// of Command or URL should be set, matching mcp.ServerConfig's own
// transport rule. Trusted is hand-side only (not part of
// mcp.ServerConfig) — it controls approval gating for this server's
// tools, not the connection itself.
type MCPServer struct {
	Name    string            `json:"name"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	// Trusted skips approval gating for every tool this server exposes.
	// Default false: an MCP tool is gated like bash/write_file/edit_file
	// until its server is marked trusted.
	Trusted bool `json:"trusted,omitempty"`
}

// DefaultPath returns ~/.hand/config.json for the current user.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".hand", "config.json"), nil
}

// Load reads the config at path. If the file does not exist, Load
// creates it (and any missing parent directories) with DefaultModel
// and returns that default.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		cfg := Config{Model: DefaultModel}
		if saveErr := Save(path, cfg); saveErr != nil {
			return Config{}, fmt.Errorf("write default config: %w", saveErr)
		}
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	// Tighten permissions on a config file that predates the 0o600
	// change in Save (or was otherwise created/edited with looser
	// permissions) — this file can carry MCP bearer tokens, so bring it
	// in line every time it's loaded, not just when hand itself writes it.
	if err := os.Chmod(path, 0o600); err != nil {
		return Config{}, fmt.Errorf("tighten permissions on config %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

// Save writes cfg to path as indented JSON, creating any missing
// parent directories.
func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	// 0o600: MCPServers entries may carry bearer tokens or other secrets
	// in Headers/Env, so this file is treated like a credential file —
	// unreadable by other local users on a shared machine.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}

// ResolveModel returns flagValue if non-empty, otherwise cfg.Model.
// Does not persist anything — a --model flag overrides the loaded
// config for this invocation only.
func ResolveModel(flagValue string, cfg Config) string {
	if flagValue != "" {
		return flagValue
	}
	return cfg.Model
}

// ResolveBaseURL returns flagValue if non-empty, otherwise cfg.BaseURL.
// Does not persist anything — a --base-url flag overrides the loaded
// config for this invocation only. An empty result means "use the
// provider's own default endpoint".
func ResolveBaseURL(flagValue string, cfg Config) string {
	if flagValue != "" {
		return flagValue
	}
	return cfg.BaseURL
}

// ResolveMaxTurns returns flagValue if positive, otherwise cfg.MaxTurns
// if positive, otherwise DefaultMaxTurns. Does not persist anything — a
// --max-turns flag overrides the loaded config for this invocation only.
func ResolveMaxTurns(flagValue int, cfg Config) int {
	if flagValue > 0 {
		return flagValue
	}
	if cfg.MaxTurns > 0 {
		return cfg.MaxTurns
	}
	return DefaultMaxTurns
}

// ResolveFallbackModel returns flagValue if non-empty, otherwise
// cfg.FallbackModel. Does not persist anything — a --fallback-model flag
// overrides the loaded config for this invocation only. An empty result
// means "no fallback".
func ResolveFallbackModel(flagValue string, cfg Config) string {
	if flagValue != "" {
		return flagValue
	}
	return cfg.FallbackModel
}

// ToServerConfigs converts cfg.MCPServers to harness's mcp.ServerConfig
// type, dropping the hand-only Trusted flag (approval gating, handled
// separately by TrustedMCPServers, has no place in mcp.ServerConfig).
func (cfg Config) ToServerConfigs() []mcp.ServerConfig {
	if len(cfg.MCPServers) == 0 {
		return nil
	}
	servers := make([]mcp.ServerConfig, len(cfg.MCPServers))
	for i, s := range cfg.MCPServers {
		servers[i] = mcp.ServerConfig{
			Name:    s.Name,
			Command: s.Command,
			Args:    s.Args,
			Env:     s.Env,
			URL:     s.URL,
			Headers: s.Headers,
		}
	}
	return servers
}

// TrustedMCPServers returns the set of server names marked Trusted, for
// NewApprovalHook/NewOneShotApprovalHook's gating check. A server absent
// from cfg.MCPServers (or present but not trusted) is simply not a key
// in the returned map.
func (cfg Config) TrustedMCPServers() map[string]bool {
	trusted := make(map[string]bool)
	for _, s := range cfg.MCPServers {
		if s.Trusted {
			trusted[s.Name] = true
		}
	}
	return trusted
}

// AllMCPServerNames returns every configured server's Name, trusted or
// not. NewApprovalHook/NewOneShotApprovalHook need the *complete* set
// (not just TrustedMCPServers) to correctly split a harness adapter tool
// name "mcp__<Name>__<tool>" when a server's own Name contains "__" —
// matching against the longest known name (trusted or not) is the only
// way to avoid mistaking, say, trusted server "brave" for the real,
// untrusted server "brave__search" when both share that prefix.
func (cfg Config) AllMCPServerNames() []string {
	if len(cfg.MCPServers) == 0 {
		return nil
	}
	names := make([]string, len(cfg.MCPServers))
	for i, s := range cfg.MCPServers {
		names[i] = s.Name
	}
	return names
}
