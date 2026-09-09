package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/sausheong/harness/runtime"
	"os"
	"path/filepath"
	"testing"

	"github.com/sausheong/hand/internal/extensions"
)

func TestExtensionStartupRequiresExactApprovalAndBoundary(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "extensions.json")
	selected := ExtensionStartup{Version: 1, SnapshotRoot: filepath.Join(dir, "private"), Identities: map[string]string{"note": "package/note"}, Reviews: []extensions.LaunchReview{{Specification: extensions.Specification{Name: "note"}, Workspace: dir}}}
	raw, _ := json.Marshal(selected)
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	for _, approval := range []string{"", "wrong", string(make([]byte, 64))} {
		if _, err := ReadExtensionStartup(file, approval); err == nil {
			t.Fatal("unapproved configuration accepted")
		}
	}
	if _, err := ReadExtensionStartup(file, digest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, append(raw, ' '), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadExtensionStartup(file, digest); err == nil {
		t.Fatal("changed approved bytes accepted")
	}
	duplicate := []byte(`{"version":1,"version":1}`)
	sum = sha256.Sum256(duplicate)
	if err := os.WriteFile(file, duplicate, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadExtensionStartup(file, hex.EncodeToString(sum[:])); err == nil {
		t.Fatal("duplicate config accepted")
	}
	controller := &Controller{}
	if _, err := controller.ActivateExtensions(context.Background(), selected, dir, false); err == nil {
		t.Fatal("host fallback accepted")
	}
	if _, err := controller.ActivateExtensions(context.Background(), selected, t.TempDir(), true); err == nil {
		t.Fatal("different workspace accepted")
	}
	if _, err := os.Stat(selected.SnapshotRoot); !os.IsNotExist(err) {
		t.Fatal("rejected activation created snapshots")
	}
}

// A second activation must not stack hooks referring to a retired host. All
// reconfiguration goes through the existing host's transactional reload.
func TestExtensionActivationOwnsRuntimeAndCannotStackHooks(t *testing.T) {
	c := &Controller{Rt: &runtime.Runtime{}}
	workspace := t.TempDir()
	selected := ExtensionStartup{Version: 1, SnapshotRoot: filepath.Join(t.TempDir(), "snapshots")}
	ctx := context.Background()
	_, release, err := c.owner().reserve(ctx, Running)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.ActivateExtensions(ctx, selected, workspace, true); !errors.Is(err, ErrBusy) {
		t.Fatalf("busy activation: %v", err)
	}
	release()
	if _, err = os.Stat(selected.SnapshotRoot); !os.IsNotExist(err) {
		t.Fatal("busy activation created resources")
	}
	// A failed admission must leave a subsequent valid activation possible.
	if _, err = c.ActivateExtensions(ctx, selected, workspace, false); err == nil {
		t.Fatal("boundary refusal missing")
	}
	host, err := c.ActivateExtensions(ctx, selected, workspace, true)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	permission := c.Rt.Permission
	for _, closed := range []bool{false, true} {
		if closed {
			if err = host.Close(); err != nil {
				t.Fatal(err)
			}
		}
		other := selected
		other.SnapshotRoot = filepath.Join(t.TempDir(), "must-not-create")
		if _, err = c.ActivateExtensions(ctx, other, workspace, true); err == nil {
			t.Fatal("repeat activation accepted")
		}
		if c.Rt.Permission != permission {
			t.Fatal("repeat activation replaced runtime permission")
		}
		if _, err = os.Stat(other.SnapshotRoot); !os.IsNotExist(err) {
			t.Fatal("repeat activation created resources")
		}
	}
	// Refusal must release the shared owner for subsequent core operations.
	_, release, err = c.owner().reserve(ctx, Running)
	if err != nil {
		t.Fatal(err)
	}
	release()
}
