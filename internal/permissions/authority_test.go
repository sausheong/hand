//go:build darwin || linux

package permissions

import (
	"os"
	"path/filepath"
	"testing"
)

func persistentFixture(ctx AuthorityContext) ScopedGrant {
	g := grantFixture(ctx)
	g.Lifetime = PersistentGrant
	g.SessionID = ""
	g.InvocationID = ""
	return g
}
func TestAuthorityPersistRevokeAndExclusiveOwnership(t *testing.T) {
	_, ctx := scopedFixture(t)
	dir := authorityFixtureDir(t)
	a, err := OpenAuthority(dir, ctx.Workspace, ctx.ConfigDigest)
	if err != nil {
		t.Fatal(err)
	}
	g := persistentFixture(ctx)
	if err = a.Grant(g); err != nil {
		t.Fatal(err)
	}
	if other, err := OpenAuthority(dir, ctx.Workspace, ctx.ConfigDigest); err == nil {
		other.Close()
		t.Fatal("duplicate authority writer")
	}
	a.Close()
	a, err = OpenAuthority(dir, ctx.Workspace, ctx.ConfigDigest)
	if err != nil {
		t.Fatal(err)
	}
	req := AccessRequest{FileWrite, "new.txt"}
	if !a.Allowed(ctx, req) {
		t.Fatal("durable grant absent")
	}
	if err = a.Revoke(g.ID); err != nil {
		t.Fatal(err)
	}
	a.Close()
	a, err = OpenAuthority(dir, ctx.Workspace, ctx.ConfigDigest)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if a.Allowed(ctx, req) || len(a.Grants()) != 0 {
		t.Fatal("revocation lost")
	}
}
func TestAuthorityRejectsProjectStorageAndTruncation(t *testing.T) {
	_, ctx := scopedFixture(t)
	if a, err := OpenAuthority(filepath.Join(ctx.Workspace, ".hand", "authority"), ctx.Workspace, ctx.ConfigDigest); err == nil {
		a.Close()
		t.Fatal("project storage accepted")
	}
	dir := authorityFixtureDir(t)
	if err := os.WriteFile(filepath.Join(dir, "grants.jsonl"), []byte(`{"version":1`), 0600); err != nil {
		t.Fatal(err)
	}
	if a, err := OpenAuthority(dir, ctx.Workspace, ctx.ConfigDigest); err == nil {
		a.Close()
		t.Fatal("truncated authority accepted")
	}
}
func TestAuthorityChangedResourceRequiresReapproval(t *testing.T) {
	_, ctx := scopedFixture(t)
	target := filepath.Join(ctx.Workspace, "allowed")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	dir := authorityFixtureDir(t)
	a, err := OpenAuthority(dir, ctx.Workspace, ctx.ConfigDigest)
	if err != nil {
		t.Fatal(err)
	}
	g := persistentFixture(ctx)
	g.Resource = target
	if err = a.Grant(g); err != nil {
		t.Fatal(err)
	}
	a.Close()
	if err = os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(t.TempDir(), target); err != nil {
		t.Fatal(err)
	}
	if a, err = OpenAuthority(dir, ctx.Workspace, ctx.ConfigDigest); err == nil {
		a.Close()
		t.Fatal("saved path silently expanded through symlink")
	}
}
func TestAuthorityPersistenceFailureDoesNotGrant(t *testing.T) {
	_, ctx := scopedFixture(t)
	a, err := OpenAuthority(authorityFixtureDir(t), ctx.Workspace, ctx.ConfigDigest)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.file.Close()
	if err = a.Grant(persistentFixture(ctx)); err == nil {
		t.Fatal("write failure ignored")
	}
	if a.Allowed(ctx, AccessRequest{FileWrite, "file"}) {
		t.Fatal("failed grant became active")
	}
}

func authorityFixtureDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "private-authority")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAuthorityRevokedResourceDoesNotBlockReopen(t *testing.T) {
	_, ctx := scopedFixture(t)
	dir := authorityFixtureDir(t)
	resource := filepath.Join(ctx.Workspace, "revoked")
	if err := os.Mkdir(resource, 0700); err != nil {
		t.Fatal(err)
	}
	a, err := OpenAuthority(dir, ctx.Workspace, ctx.ConfigDigest)
	if err != nil {
		t.Fatal(err)
	}
	g := persistentFixture(ctx)
	g.Resource = resource
	if err = a.Grant(g); err != nil {
		t.Fatal(err)
	}
	if err = a.Revoke(g.ID); err != nil {
		t.Fatal(err)
	}
	live := persistentFixture(ctx)
	live.ID = "live"
	live.Resource = filepath.Join(ctx.Workspace, "live")
	if err = a.Grant(live); err != nil {
		t.Fatal(err)
	}
	a.Close()
	if err = os.Remove(resource); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(t.TempDir(), resource); err != nil {
		t.Fatal(err)
	}
	a, err = OpenAuthority(dir, ctx.Workspace, ctx.ConfigDigest)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if !a.Allowed(ctx, AccessRequest{FileWrite, "live/new.txt"}) {
		t.Fatal("unrelated live grant lost")
	}
	if a.Allowed(ctx, AccessRequest{FileWrite, "revoked/new.txt"}) {
		t.Fatal("revoked authority returned")
	}
}
