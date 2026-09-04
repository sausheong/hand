// Package sessionio maps a Hand workspace directory onto a
// harness session.Store key, and resolves where sessions live on disk.
package sessionio

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// StoreDir returns ~/.hand/sessions for the current user.
func StoreDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".hand", "sessions"), nil
}

// KeyForWorkspace returns a stable, harness session.Store-safe key for
// workspace: a 16-hex-character SHA-256 prefix of the path. A hash
// (rather than a sanitized slug) avoids two real problems a slug has:
// collisions between different paths that sanitize to the same string,
// and filenames blowing past reasonable length limits for deeply nested
// projects. It also trivially satisfies the store's requirement that a
// key be a single path component with no separators.
func KeyForWorkspace(workspace string) string {
	sum := sha256.Sum256([]byte(workspace))
	return hex.EncodeToString(sum[:])[:16]
}
