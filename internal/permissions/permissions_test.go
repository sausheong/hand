package permissions_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sausheong/agcode/internal/permissions"
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
	path := filepath.Join(t.TempDir(), ".agcode", "settings.json")
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
	path := filepath.Join(t.TempDir(), ".agcode", "settings.json")

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
	path := filepath.Join(t.TempDir(), ".agcode", "settings.json")
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
