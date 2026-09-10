//go:build darwin || linux

package permissions

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthorityRejectsUnsafeStorageWithoutChangingEvidence(t *testing.T) {
	for _, name := range []string{"public-directory", "public-file", "symlink", "directory-file", "oversized-file", "oversized-record", "trailing-json", "malformed-json"} {
		t.Run(name, func(t *testing.T) {
			_, ctx := scopedFixture(t)
			dir := authorityFixtureDir(t)
			path := filepath.Join(dir, "grants.jsonl")
			a, err := OpenAuthority(dir, ctx.Workspace, ctx.ConfigDigest)
			if err != nil {
				t.Fatal(err)
			}
			if err = a.Grant(persistentFixture(ctx)); err != nil {
				t.Fatal(err)
			}
			if err = a.Close(); err != nil {
				t.Fatal(err)
			}
			valid, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(t.TempDir(), "sentinel")
			switch name {
			case "public-directory":
				err = os.Chmod(dir, 0755)
			case "public-file":
				err = os.Chmod(path, 0644)
			case "symlink":
				if err = os.WriteFile(outside, valid, 0600); err != nil {
					t.Fatal(err)
				}
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
				err = os.Symlink(outside, path)
			case "directory-file":
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
				err = os.Mkdir(path, 0700)
			case "oversized-file":
				err = os.WriteFile(path, bytes.Repeat([]byte{'\n'}, MaxAuthorityBytes+1), 0600)
			case "oversized-record":
				err = os.WriteFile(path, append(bytes.Repeat([]byte{' '}, MaxAuthorityRecordBytes), '\n'), 0600)
			case "trailing-json":
				err = os.WriteFile(path, append(append(bytes.Clone(bytes.TrimSpace(valid)), []byte(` {}`)...), '\n'), 0600)
			case "malformed-json":
				err = os.WriteFile(path, []byte("{broken}\n"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			beforeInfo, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			var before []byte
			if !beforeInfo.IsDir() {
				before, err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			got, err := OpenAuthority(dir, ctx.Workspace, ctx.ConfigDigest)
			if got != nil {
				got.Close()
			}
			if err == nil || got != nil {
				t.Fatal("unsafe storage published authority")
			}
			afterInfo, err := os.Lstat(path)
			if err != nil || beforeInfo.Mode() != afterInfo.Mode() {
				t.Fatal("storage mode changed", err)
			}
			if !beforeInfo.IsDir() {
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("evidence changed", err)
				}
			}
			if name == "symlink" {
				target, err := os.Readlink(path)
				if err != nil || target != outside {
					t.Fatal("symlink changed", err)
				}
			}
			if err = os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if name == "symlink" || name == "directory-file" {
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			if err = os.WriteFile(path, valid, 0600); err != nil {
				t.Fatal(err)
			}
			if err = os.Chmod(path, 0600); err != nil {
				t.Fatal(err)
			}
			a, err = OpenAuthority(dir, ctx.Workspace, ctx.ConfigDigest)
			if err != nil {
				t.Fatal("failed open leaked ownership", err)
			}
			defer a.Close()
			if !a.Allowed(ctx, AccessRequest{FileWrite, "new.txt"}) {
				t.Fatal("authentic authority lost")
			}
		})
	}
}
