package permissions_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sausheong/hand/internal/permissions"
)

func TestLoad_MissingFileReturnsEmptyWithoutCreating(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")

	got, err := permissions.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(got.AlwaysAllow) != 0 {
		t.Fatalf("AlwaysAllow = %v, want empty", got.AlwaysAllow)
	}

	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatal("Load must not create a settings file for a missing path")
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".hand", "settings.json")
	want := permissions.Settings{AlwaysAllow: []string{"bash", "write_file"}}

	if err := permissions.Save(path, want); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	got, err := permissions.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(got.AlwaysAllow) != 2 || got.AlwaysAllow[0] != "bash" || got.AlwaysAllow[1] != "write_file" {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestSettings_IsAlwaysAllowed(t *testing.T) {
	s := permissions.Settings{AlwaysAllow: []string{"bash"}}
	if !s.IsAlwaysAllowed("bash") {
		t.Error("expected bash to be always-allowed")
	}
	if s.IsAlwaysAllowed("write_file") {
		t.Error("expected write_file to not be always-allowed")
	}
}

func TestStore_SetAlwaysAllow_PersistsAndReflectsAcrossStores(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".hand", "settings.json")

	st1, err := permissions.NewStore(path)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	if st1.IsAlwaysAllowed("bash") {
		t.Fatal("bash should not be always-allowed before SetAlwaysAllow")
	}
	if err := st1.SetAlwaysAllow("bash"); err != nil {
		t.Fatalf("SetAlwaysAllow returned error: %v", err)
	}
	if !st1.IsAlwaysAllowed("bash") {
		t.Fatal("bash should be always-allowed immediately after SetAlwaysAllow, in the same Store")
	}

	// A second Store opened against the same path must see the persisted change.
	st2, err := permissions.NewStore(path)
	if err != nil {
		t.Fatalf("second NewStore returned error: %v", err)
	}
	if !st2.IsAlwaysAllowed("bash") {
		t.Fatal("bash should be always-allowed in a fresh Store reading the same file")
	}
}

func TestStore_SetAlwaysAllow_NoOpWhenAlreadyAllowed(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".hand", "settings.json")
	st, err := permissions.NewStore(path)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	if err := st.SetAlwaysAllow("bash"); err != nil {
		t.Fatalf("first SetAlwaysAllow returned error: %v", err)
	}
	if err := st.SetAlwaysAllow("bash"); err != nil {
		t.Fatalf("second SetAlwaysAllow returned error: %v", err)
	}

	got, err := permissions.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(got.AlwaysAllow) != 1 {
		t.Fatalf("AlwaysAllow = %v, want exactly one entry (no duplicate)", got.AlwaysAllow)
	}
}

// Regression: always_allow entries let a tool bypass the approval
// prompt, so .hand/settings.json is a credential-equivalent file — it
// must not be readable by other local users on a shared machine.
func TestSave_WritesOwnerOnlyPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't apply on windows")
	}
	path := filepath.Join(t.TempDir(), ".hand", "settings.json")
	if err := permissions.Save(path, permissions.Settings{AlwaysAllow: []string{"bash"}}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("settings file permissions = %o, want 0600", perm)
	}
}

// Regression: an existing settings file from before Save started using
// 0o600 (or otherwise loosened by hand) should be tightened the next
// time it's loaded, not left world/group-readable indefinitely.
func TestLoad_TightensLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't apply on windows")
	}
	path := filepath.Join(t.TempDir(), ".hand", "settings.json")
	if err := permissions.Save(path, permissions.Settings{AlwaysAllow: []string{"bash"}}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("Chmod returned error: %v", err)
	}

	if _, err := permissions.Load(path); err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("settings file permissions after Load = %o, want 0600", perm)
	}
}

func TestStore_AlwaysAllowList(t *testing.T) {
	st := permissions.NewEmptyStore(filepath.Join(t.TempDir(), "settings.json"))
	if got := st.AlwaysAllowList(); len(got) != 0 {
		t.Fatalf("AlwaysAllowList on empty store = %v, want empty", got)
	}
	if err := st.SetAlwaysAllow("bash"); err != nil {
		t.Fatalf("SetAlwaysAllow returned error: %v", err)
	}
	if err := st.SetAlwaysAllow("write_file"); err != nil {
		t.Fatalf("SetAlwaysAllow returned error: %v", err)
	}
	got := st.AlwaysAllowList()
	if len(got) != 2 || got[0] != "bash" || got[1] != "write_file" {
		t.Fatalf("AlwaysAllowList() = %v, want [bash write_file]", got)
	}
}

func TestNewStoreFromSettings_UsesGivenSettingsNotDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".hand", "settings.json")
	if err := permissions.Save(path, permissions.Settings{AlwaysAllow: []string{"bash"}}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	// A caller that decided (e.g. via a trust prompt) to ignore what's on
	// disk passes its own Settings instead of re-reading the file.
	st := permissions.NewStoreFromSettings(path, permissions.Settings{})
	if st.IsAlwaysAllowed("bash") {
		t.Fatal("NewStoreFromSettings should use the given Settings, not reload path from disk")
	}

	// SetAlwaysAllow still persists to path normally afterward.
	if err := st.SetAlwaysAllow("write_file"); err != nil {
		t.Fatalf("SetAlwaysAllow returned error: %v", err)
	}
	got, err := permissions.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(got.AlwaysAllow) != 1 || got.AlwaysAllow[0] != "write_file" {
		t.Fatalf("AlwaysAllow on disk = %v, want [write_file] (the pre-existing \"bash\" entry should be gone, overwritten by the fresh store)", got.AlwaysAllow)
	}
}

func TestNewEmptyStore_IgnoresExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".hand", "settings.json")
	if err := permissions.Save(path, permissions.Settings{AlwaysAllow: []string{"bash"}}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	st := permissions.NewEmptyStore(path)
	if st.IsAlwaysAllowed("bash") {
		t.Fatal("NewEmptyStore should not honor pre-existing always_allow entries on disk")
	}
}
