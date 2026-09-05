package permissions_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sausheong/hand/internal/permissions"
)

func TestLoadTrust_MissingFileReturnsEmptyWithoutCreating(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "trust.json")

	got, err := permissions.LoadTrust(path)
	if err != nil {
		t.Fatalf("LoadTrust returned error: %v", err)
	}
	if got.IsTrusted("/some/workspace") {
		t.Fatal("an empty trust file must not trust anything")
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatal("LoadTrust must not create a trust file for a missing path")
	}
}

func TestMarkTrusted_PersistsAndReflectsAcrossLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".hand", "trust.json")
	workspace := "/home/user/project"

	if err := permissions.MarkTrusted(path, workspace); err != nil {
		t.Fatalf("MarkTrusted returned error: %v", err)
	}

	got, err := permissions.LoadTrust(path)
	if err != nil {
		t.Fatalf("LoadTrust returned error: %v", err)
	}
	if !got.IsTrusted(workspace) {
		t.Fatalf("workspace %q should be trusted after MarkTrusted", workspace)
	}
	if got.IsTrusted("/some/other/project") {
		t.Fatal("a different workspace must not be trusted")
	}
}

func TestMarkTrusted_NoOpWhenAlreadyTrusted(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".hand", "trust.json")
	workspace := "/home/user/project"

	if err := permissions.MarkTrusted(path, workspace); err != nil {
		t.Fatalf("first MarkTrusted returned error: %v", err)
	}
	if err := permissions.MarkTrusted(path, workspace); err != nil {
		t.Fatalf("second MarkTrusted returned error: %v", err)
	}

	got, err := permissions.LoadTrust(path)
	if err != nil {
		t.Fatalf("LoadTrust returned error: %v", err)
	}
	if len(got.TrustedWorkspaces) != 1 {
		t.Fatalf("TrustedWorkspaces = %v, want exactly one entry (no duplicate)", got.TrustedWorkspaces)
	}
}

// Regression: trust.json records which workspaces get to skip hand's
// approval gate without a prompt — it must not be world/group-readable.
func TestSaveTrust_WritesOwnerOnlyPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't apply on windows")
	}
	path := filepath.Join(t.TempDir(), ".hand", "trust.json")
	if err := permissions.MarkTrusted(path, "/home/user/project"); err != nil {
		t.Fatalf("MarkTrusted returned error: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("trust file permissions = %o, want 0600", perm)
	}
}
