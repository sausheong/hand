// Package config reads and writes Hand's user-level configuration
// file at ~/.hand/config.json.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultModel is used when no config file exists yet.
const DefaultModel = "anthropic/claude-sonnet-5"

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
	if err := os.WriteFile(path, data, 0o644); err != nil {
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
