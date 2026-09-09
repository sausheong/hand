package packages

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveInstalledSelectionRevalidatesContentAndIdentity(t *testing.T) {
	for _, mode := range []string{"valid", "content", "object-link", "version-metadata", "development-version", "cancelled", "duplicate", "missing"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			source := writeFixture(t)
			dir := privateStageParent(t)
			store, err := OpenStore(dir, "1.0.0")
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			_, pin, err := VerifyDirectory(ctx, source)
			if err != nil {
				t.Fatal(err)
			}
			review, err := store.PrepareInstall(ctx, source, pin)
			if err != nil {
				t.Fatal(err)
			}
			approveChange(t, store, review)
			names := []string{review.Name}
			object := filepath.Join(dir, "objects", pin)
			switch mode {
			case "content":
				file := filepath.Join(object, "note.py")
				if err = os.Chmod(file, 0600); err == nil {
					err = os.WriteFile(file, []byte("corrupt"), 0600)
				}
			case "object-link":
				saved := filepath.Join(t.TempDir(), "saved")
				if err = os.Rename(object, saved); err == nil {
					err = os.Symlink(saved, object)
				}
			case "version-metadata":
				state, e := store.List()
				if e != nil {
					t.Fatal(e)
				}
				state.Packages[0].Revisions[0].Version = "9.0.0"
				raw, e := json.Marshal(state)
				if e != nil {
					t.Fatal(e)
				}
				err = os.WriteFile(filepath.Join(dir, "lock.json"), raw, 0600)
			case "development-version":
				store.handVersion = "dev"
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "duplicate":
				names = append(names, review.Name)
			case "missing":
				names = append(names, "missing")
			}
			if err != nil {
				t.Fatal(err)
			}
			selected, err := store.ResolveSelected(ctx, names)
			if mode != "valid" {
				if err == nil || len(selected.Packages) != 0 {
					t.Fatal("invalid selection partially exposed", selected, err)
				}
				return
			}
			if err != nil || selected.Generation != 1 || len(selected.Packages) != 1 || selected.Packages[0].Digest != pin {
				t.Fatal(selected, err)
			}
			selected.Packages[0].Manifest.Extensions[0].Capabilities[0] = "changed"
			fresh, err := store.ResolveSelected(ctx, names)
			if err != nil || fresh.Packages[0].Manifest.Extensions[0].Capabilities[0] == "changed" {
				t.Fatal("resolution leaked mutable inventory", err)
			}
		})
	}
}
