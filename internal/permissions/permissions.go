// Package permissions manages agcode's project-local "always allow"
// allowlist at .agcode/settings.json.
package permissions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

// Settings is the on-disk shape of .agcode/settings.json.
type Settings struct {
	// AlwaysAllow lists tool names the user has approved with "always"
	// for this project — every future call to that tool is allowed
	// without prompting. Granularity is per tool name, not per exact
	// command.
	AlwaysAllow []string `json:"always_allow"`
}

// IsAlwaysAllowed reports whether tool is in AlwaysAllow.
func (s Settings) IsAlwaysAllowed(tool string) bool {
	return slices.Contains(s.AlwaysAllow, tool)
}

// DefaultPath returns workspace/.agcode/settings.json.
func DefaultPath(workspace string) string {
	return filepath.Join(workspace, ".agcode", "settings.json")
}

// Load reads the settings at path. Unlike internal/config.Load, a
// missing file is not an error and is not created — it returns an
// empty Settings{}. A project shouldn't get a .agcode/settings.json
// littered into it just for running agcode; the file appears only the
// first time the user actually approves something with "always".
func Load(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("read settings %s: %w", path, err)
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return Settings{}, fmt.Errorf("parse settings %s: %w", path, err)
	}
	return s, nil
}

// Save writes s to path as indented JSON, creating any missing parent
// directories.
func Save(path string, s Settings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write settings %s: %w", path, err)
	}
	return nil
}

// Store wraps Settings with the path it was loaded from and a mutex, so
// concurrent callers can safely check and update the allowlist. Load()s
// once at construction and keeps an in-memory copy in sync with disk.
type Store struct {
	path string

	mu       sync.Mutex
	settings Settings
}

// NewStore loads the settings at path (see Load) and returns a Store
// backed by it.
func NewStore(path string) (*Store, error) {
	s, err := Load(path)
	if err != nil {
		return nil, err
	}
	return &Store{path: path, settings: s}, nil
}

// IsAlwaysAllowed reports whether tool is currently always-allowed.
func (st *Store) IsAlwaysAllowed(tool string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.settings.IsAlwaysAllowed(tool)
}

// SetAlwaysAllow adds tool to the always-allow list and persists it. A
// no-op (returns nil without writing) if tool is already always-allowed.
func (st *Store) SetAlwaysAllow(tool string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.settings.IsAlwaysAllowed(tool) {
		return nil
	}
	st.settings.AlwaysAllow = append(st.settings.AlwaysAllow, tool)
	return Save(st.path, st.settings)
}
