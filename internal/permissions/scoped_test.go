package permissions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func scopedFixture(t *testing.T) (*ScopedPolicy, AuthorityContext) {
	t.Helper()
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := AuthorityContext{Workspace: workspace, ConfigDigest: strings.Repeat("a", 64), SessionID: "session", InvocationID: "invocation"}
	p, err := NewScopedPolicy(workspace, ctx.ConfigDigest)
	if err != nil {
		t.Fatal(err)
	}
	return p, ctx
}
func grantFixture(ctx AuthorityContext) ScopedGrant {
	return ScopedGrant{ID: "grant", Operation: FileWrite, Scope: TreeScope, Resource: ".", Lifetime: InvocationGrant, SessionID: ctx.SessionID, InvocationID: ctx.InvocationID, Provenance: UserDecision, Workspace: ctx.Workspace, ConfigDigest: ctx.ConfigDigest}
}
func TestScopedGrantLifetimeConfigAndRevocation(t *testing.T) {
	p, ctx := scopedFixture(t)
	g := grantFixture(ctx)
	if err := p.Grant(g); err != nil {
		t.Fatal(err)
	}
	req := AccessRequest{Operation: FileWrite, Resource: "new/sub/file.go"}
	if !p.Allowed(ctx, req) {
		t.Fatal("scoped write denied")
	}
	for _, changed := range []AuthorityContext{{Workspace: ctx.Workspace, ConfigDigest: ctx.ConfigDigest, SessionID: ctx.SessionID, InvocationID: "other"}, {Workspace: ctx.Workspace, ConfigDigest: strings.Repeat("b", 64), SessionID: ctx.SessionID, InvocationID: ctx.InvocationID}, {Workspace: ctx.Workspace, ConfigDigest: ctx.ConfigDigest, SessionID: "other", InvocationID: ctx.InvocationID}} {
		if p.Allowed(changed, req) {
			t.Fatal("authority expanded")
		}
	}
	copy := p.Grants()
	copy[0].Resource = "/"
	if p.Grants()[0].Resource != ctx.Workspace {
		t.Fatal("inspection mutated authority")
	}
	if !p.Revoke(g.ID) || p.Allowed(ctx, req) {
		t.Fatal("revoked grant still applies")
	}
}
func TestScopedGrantSymlinkAndPrefixEscape(t *testing.T) {
	p, ctx := scopedFixture(t)
	dir := filepath.Join(ctx.Workspace, "allowed")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	g := grantFixture(ctx)
	g.Resource = dir
	if err := p.Grant(g); err != nil {
		t.Fatal(err)
	}
	for _, resource := range []string{"allowed/escape/new.txt", "allowed-other/new.txt", "../outside/new.txt"} {
		if p.Allowed(ctx, AccessRequest{FileWrite, resource}) {
			t.Fatalf("escaped grant: %s", resource)
		}
	}
	if !p.Allowed(ctx, AccessRequest{FileWrite, "allowed/new.txt"}) {
		t.Fatal("normal child denied")
	}
	if err := os.Symlink(filepath.Join(ctx.Workspace, "missing"), filepath.Join(dir, "dangling")); err != nil {
		t.Fatal(err)
	}
	if p.Allowed(ctx, AccessRequest{FileWrite, "allowed/dangling/new.txt"}) {
		t.Fatal("unresolved symlink accepted")
	}
}
func TestScopedCommandAndMCPExactIdentity(t *testing.T) {
	p, ctx := scopedFixture(t)
	for _, tc := range []struct {
		op              Operation
		resource, other string
	}{{CommandExec, `["go","test","./..."]`, `["go","test","./...",";","touch","outside"]`}, {ShellExec, "go test ./...", "go test ./...; touch outside"}, {MCPCall, `["server-a","write"]`, `["server-b","write"]`}, {NetworkAccess, "https://example.com", "https://example.com.evil"}} {
		g := grantFixture(ctx)
		g.ID = string(tc.op)
		g.Operation = tc.op
		g.Scope = ExactScope
		g.Resource = tc.resource
		if err := p.Grant(g); err != nil {
			t.Fatal(err)
		}
		if !p.Allowed(ctx, AccessRequest{tc.op, tc.resource}) || p.Allowed(ctx, AccessRequest{tc.op, tc.other}) {
			t.Fatalf("exact resource mismatch %s", tc.op)
		}
	}
}
func TestScopedProjectProvenanceRejected(t *testing.T) {
	p, ctx := scopedFixture(t)
	g := grantFixture(ctx)
	g.Provenance = "project_policy"
	if err := p.Grant(g); err == nil {
		t.Fatal("project policy became authority")
	}
	g.Provenance = UserDecision
	g.Lifetime = PersistentGrant
	if err := p.Grant(g); err == nil {
		t.Fatal("ambiguous persistent lifetime accepted")
	}
}
