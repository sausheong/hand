package permissions_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/sausheong/hand/internal/permissions"
)

// Legacy stores are migration input. A partially decoded grant must not escape
// alongside a parse/read error, nor may a retry overwrite unreadable evidence.
func TestLegacyTrustReadFailureCannotGrantOrOverwrite(t *testing.T) {
	for _, damaged := range []string{`{"trusted_workspaces":["/approved"],"broken":`, `{"trusted_workspaces":["/approved",42]}`} {
		t.Run(damaged, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "trust.json")
			if err := os.WriteFile(path, []byte(damaged), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := permissions.LoadTrust(path)
			if err == nil || got.IsTrusted("/approved") || len(got.TrustedWorkspaces) != 0 {
				t.Fatalf("partial trust escaped: %+v, %v", got, err)
			}
			if err := permissions.MarkTrusted(path, "/new"); err == nil {
				t.Fatal("mark replaced damaged trust")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(after, []byte(damaged)) {
				t.Fatal("failed mark changed evidence")
			}
		})
	}
	path := t.TempDir()
	got, err := permissions.LoadTrust(path)
	if err == nil || len(got.TrustedWorkspaces) != 0 {
		t.Fatal("directory accepted as trust")
	}
	if err := permissions.MarkTrusted(path, "/new"); err == nil {
		t.Fatal("mark accepted directory")
	}
}

func TestLegacySettingsReadFailureCannotGrant(t *testing.T) {
	for _, damaged := range []string{`{"always_allow":["bash"],"broken":`, `{"always_allow":["bash",42]}`} {
		t.Run(damaged, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if err := os.WriteFile(path, []byte(damaged), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := permissions.Load(path)
			if err == nil || got.IsAlwaysAllowed("bash") || len(got.AlwaysAllow) != 0 {
				t.Fatalf("partial grant escaped: %+v, %v", got, err)
			}
			store, err := permissions.NewStore(path)
			if err == nil || store != nil {
				t.Fatal("damaged settings opened usable store")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(after, []byte(damaged)) {
				t.Fatal("load changed damaged settings bytes")
			}
		})
	}
	got, err := permissions.Load(t.TempDir())
	if err == nil || len(got.AlwaysAllow) != 0 {
		t.Fatal("directory accepted as settings")
	}
}

func TestLegacyStoreSaveFailurePreservesObstruction(t *testing.T) {
	for _, kind := range []string{"trust", "settings"} {
		for _, obstruction := range []string{"parent_file", "target_directory"} {
			t.Run(kind+"/"+obstruction, func(t *testing.T) {
				root := t.TempDir()
				marker := []byte("must survive failed save")
				path := filepath.Join(root, "target")
				markerPath := filepath.Join(path, "marker")
				if obstruction == "parent_file" {
					markerPath = path
					if err := os.WriteFile(markerPath, marker, 0600); err != nil {
						t.Fatal(err)
					}
					path = filepath.Join(path, "store.json")
				} else {
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(markerPath, marker, 0600); err != nil {
						t.Fatal(err)
					}
				}
				var err error
				if kind == "trust" {
					err = permissions.SaveTrust(path, permissions.TrustFile{TrustedWorkspaces: []string{"/new"}})
				} else {
					err = permissions.Save(path, permissions.Settings{AlwaysAllow: []string{"bash"}})
				}
				if err == nil {
					t.Fatal("save succeeded through filesystem obstruction")
				}
				after, err := os.ReadFile(markerPath)
				if err != nil || !bytes.Equal(after, marker) {
					t.Fatal("failed save changed obstruction")
				}
			})
		}
	}
}
