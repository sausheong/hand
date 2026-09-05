package permissions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// TrustFile is the on-disk shape of ~/.hand/trust.json — the set of
// workspace directories whose .hand/settings.json always_allow entries
// the user has explicitly agreed to honor. Global (not per-project)
// because the whole point is that a project's own settings file cannot
// self-certify itself as trustworthy; the decision has to live outside
// whatever a cloned repo could ship.
type TrustFile struct {
	TrustedWorkspaces []string `json:"trusted_workspaces"`
}

// IsTrusted reports whether workspace has been marked trusted.
func (t TrustFile) IsTrusted(workspace string) bool {
	return slices.Contains(t.TrustedWorkspaces, workspace)
}

// TrustPath returns ~/.hand/trust.json for the current user.
func TrustPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".hand", "trust.json"), nil
}

// LoadTrust reads the trust file at path. A missing file is not an
// error — it returns an empty TrustFile{}, matching Load's behavior for
// .hand/settings.json (nothing is trusted until the user says so).
func LoadTrust(path string) (TrustFile, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return TrustFile{}, nil
	}
	if err != nil {
		return TrustFile{}, fmt.Errorf("read trust store %s: %w", path, err)
	}

	var t TrustFile
	if err := json.Unmarshal(data, &t); err != nil {
		return TrustFile{}, fmt.Errorf("parse trust store %s: %w", path, err)
	}
	return t, nil
}

// SaveTrust writes t to path as indented JSON, creating any missing
// parent directories.
func SaveTrust(path string, t TrustFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create trust store directory: %w", err)
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("encode trust store: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write trust store %s: %w", path, err)
	}
	return nil
}

// MarkTrusted adds workspace to the trust file at path, creating or
// updating it. A no-op if workspace is already trusted.
func MarkTrusted(path, workspace string) error {
	t, err := LoadTrust(path)
	if err != nil {
		return err
	}
	if t.IsTrusted(workspace) {
		return nil
	}
	t.TrustedWorkspaces = append(t.TrustedWorkspaces, workspace)
	return SaveTrust(path, t)
}
