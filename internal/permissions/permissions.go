// Package permissions manages Hand's project-local "always allow"
// allowlist at .hand/settings.json.
package permissions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

// Settings is the on-disk shape of .hand/settings.json.
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

// DefaultPath returns workspace/.hand/settings.json.
func DefaultPath(workspace string) string {
	return filepath.Join(workspace, ".hand", "settings.json")
}

// Load reads the settings at path. Unlike internal/config.Load, a
// missing file is not an error and is not created — it returns an
// empty Settings{}. A project shouldn't get a .hand/settings.json
// littered into it just for running hand; the file appears only the
// first time the user actually approves something with "always".
func Load(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("read settings %s: %w", path, err)
	}
	// Tighten permissions on a settings file that predates the 0o600
	// change in Save — its always_allow entries let a tool bypass an
	// approval prompt, so bring it in line every time it's loaded, not
	// just when hand itself writes it.
	if err := os.Chmod(path, 0o600); err != nil {
		return Settings{}, fmt.Errorf("tighten permissions on settings %s: %w", path, err)
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
	// 0o600: this file's always_allow entries let a tool run without a
	// human approval prompt, so it's treated the same as a credential —
	// unreadable by other local users on a shared machine, not just
	// unwritable.
	if err := os.WriteFile(path, data, 0o600); err != nil {
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

// NewStoreFromSettings returns a Store bound to path whose initial
// in-memory content is s, rather than whatever Load(path) would return.
// For callers that already loaded (and possibly vetted, e.g. against a
// workspace-trust decision) Settings themselves — see cmd/hand's
// trust-on-first-use prompt for a workspace's always_allow entries.
// SetAlwaysAllow still persists to path as normal.
func NewStoreFromSettings(path string, s Settings) *Store {
	return &Store{path: path, settings: s}
}

// NewEmptyStore returns a Store bound to path with no in-memory
// always-allow entries, regardless of what (if anything) is currently on
// disk at path. Used when a caller has decided not to honor path's
// existing content for this run (e.g. the user declined to trust it) —
// a later SetAlwaysAllow during the session still persists normally,
// starting from a clean slate.
func NewEmptyStore(path string) *Store {
	return NewStoreFromSettings(path, Settings{})
}

// IsAlwaysAllowed reports whether tool is currently always-allowed.
func (st *Store) IsAlwaysAllowed(tool string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.settings.IsAlwaysAllowed(tool)
}

// AlwaysAllowList returns a copy of the tool names currently
// always-allowed.
func (st *Store) AlwaysAllowList() []string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return slices.Clone(st.settings.AlwaysAllow)
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
