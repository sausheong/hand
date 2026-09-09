package packages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstalledTextResources(t *testing.T) {
	for _, mode := range []string{"prompt", "skill", "nested-skill", "invalid-utf8", "too-large", "unlisted", "wrong-kind", "corrupt", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			source := t.TempDir()
			body := []byte("Use the repository's verification commands.\n")
			path, kind := "review.md", "prompt"
			switch mode {
			case "skill":
				path = "SKILL.md"
				kind = "skill"
			case "nested-skill":
				path = "skills/review/SKILL.md"
				kind = "skill"
			case "invalid-utf8":
				body = []byte{0xff}
			case "too-large":
				body = []byte(strings.Repeat("x", MaxTextResourceBytes+1))
			case "wrong-kind":
				kind = "asset"
			}
			sum := sha256.Sum256(body)
			m := Manifest{Schema: 1, Name: "text-resources", Version: "1.0.0", Compatibility: Compatibility{MinimumHand: "1.0.0", ExtensionProtocol: 1}, Files: []File{{Path: path, Kind: kind, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(body))}}}
			if err := os.MkdirAll(filepath.Dir(filepath.Join(source, path)), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(source, path), body, 0600); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(source, ManifestName), raw, 0600); err != nil {
				t.Fatal(err)
			}
			_, pin, err := VerifyDirectory(ctx, source)
			if err != nil {
				t.Fatal(err)
			}
			store, err := OpenStore(filepath.Join(t.TempDir(), "store"), "1.0.0")
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			change, err := store.PrepareInstall(ctx, source, pin)
			if err != nil {
				t.Fatal(err)
			}
			approveChange(t, store, change)
			if err = os.RemoveAll(source); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "unlisted":
				path = "../review.md"
			case "corrupt":
				file := filepath.Join(store.directory, "objects", pin, path)
				if err = os.Chmod(file, 0600); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(file, []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			resource, err := store.ReadTextResource(ctx, m.Name, path)
			if mode != "prompt" && mode != "skill" && mode != "nested-skill" {
				if err == nil || resource.Text != "" {
					t.Fatal("invalid resource exposed", resource, err)
				}
				return
			}
			if err != nil || resource.Text != string(body) || resource.PackageDigest != pin || resource.Generation != 1 || resource.Kind != kind {
				t.Fatal(resource, err)
			}
			wantSource := filepath.Join(store.directory, "objects", pin, filepath.FromSlash(path))
			if resource.SourcePath != wantSource || resource.BaseDirectory != filepath.Dir(wantSource) {
				t.Fatal("resource base did not identify retained, digest-pinned source", resource)
			}
			retained, err := os.ReadFile(resource.SourcePath)
			if err != nil || string(retained) != resource.Text {
				t.Fatal("resource source no longer resolves after installation source removal", err)
			}
			batch, err := store.ReadTextResources(ctx, []TextResourceSelection{{Package: m.Name, Path: path}, {Package: m.Name, Path: path}})
			if err != nil || len(batch) != 2 || batch[0].Generation != batch[1].Generation || batch[0].Text != string(body) {
				t.Fatal(batch, err)
			}
			batch, err = store.ReadTextResources(ctx, []TextResourceSelection{{Package: m.Name, Path: path}, {Package: m.Name, Path: "missing.md"}})
			if err == nil || len(batch) != 0 {
				t.Fatal("partial text selection escaped", batch, err)
			}

		})
	}
}
