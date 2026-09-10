//go:build darwin || linux

package permissions

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthorityWriteFailureDisablesExistingGrantsUntilReopen(t *testing.T) {
	for _, operation := range []string{"grant", "revoke"} {
		t.Run(operation, func(t *testing.T) {
			_, ctx := scopedFixture(t)
			directory := authorityFixtureDir(t)
			authority, err := OpenAuthority(directory, ctx.Workspace, ctx.ConfigDigest)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = authority.Close() }()
			original := persistentFixture(ctx)
			if err := authority.Grant(original); err != nil {
				t.Fatal(err)
			}
			request := AccessRequest{FileWrite, "existing.txt"}
			if !authority.Allowed(ctx, request) {
				t.Fatal("fixture grant not active")
			}
			path := filepath.Join(directory, "grants.jsonl")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// Closing the owned descriptor produces a real OS write failure without
			// changing filesystem permissions or simulating a successful revocation.
			if err := authority.file.Close(); err != nil {
				t.Fatal(err)
			}
			added := original
			added.ID = "second"
			if operation == "grant" {
				err = authority.Grant(added)
			} else {
				err = authority.Revoke(original.ID)
			}
			if err == nil {
				t.Fatal("failed journal write reported success")
			}
			if authority.Allowed(ctx, request) || authority.AllowedTool(ctx, "write_file", request) {
				t.Fatal("poisoned authority still authorises existing grant")
			}
			if err := authority.Grant(added); err == nil {
				t.Fatal("poisoned authority accepted later grant")
			}
			if err := authority.Revoke(original.ID); err == nil {
				t.Fatal("poisoned authority accepted later revocation")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("failed writes changed durable journal")
			}
			_ = authority.Close()
			authority, err = OpenAuthority(directory, ctx.Workspace, ctx.ConfigDigest)
			if err != nil {
				t.Fatal(err)
			}
			// Restart reflects the durable history: the failed operation did not
			// revoke the original grant or persist the newly requested grant.
			grants := authority.Grants()
			if len(grants) != 1 || grants[0].ID != original.ID || !authority.Allowed(ctx, request) {
				t.Fatalf("reopen misrepresented failed write: %+v", grants)
			}
			if err := authority.Revoke(original.ID); err != nil {
				t.Fatal(err)
			}
			if authority.Allowed(ctx, request) {
				t.Fatal("successful retry did not revoke")
			}
		})
	}
}
