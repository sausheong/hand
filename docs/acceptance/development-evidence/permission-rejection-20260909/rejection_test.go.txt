package permissions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRejectedGrantsPreserveExistingAuthority(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ScopedGrant)
	}{
		{"empty-id", func(g *ScopedGrant) { g.ID = "" }},
		{"unknown-operation", func(g *ScopedGrant) { g.Operation = "future" }},
		{"unknown-scope", func(g *ScopedGrant) { g.Scope = "prefix" }},
		{"missing-invocation", func(g *ScopedGrant) { g.InvocationID = "" }},
		{"missing-session", func(g *ScopedGrant) { g.SessionID = "" }},
		{"session-with-invocation", func(g *ScopedGrant) { g.Lifetime = SessionGrant }},
		{"unknown-lifetime", func(g *ScopedGrant) { g.Lifetime = "forever" }},
		{"duplicate-id", func(g *ScopedGrant) { g.ID = "grant" }},
		{"empty-file", func(g *ScopedGrant) { g.Resource = "" }},
		{"empty-shell", func(g *ScopedGrant) { g.Operation = ShellExec; g.Resource = "" }},
		{"command-null", func(g *ScopedGrant) { g.Operation = CommandExec; g.Resource = "null" }},
		{"command-no-executable", func(g *ScopedGrant) { g.Operation = CommandExec; g.Resource = `["","arg"]` }},
		{"network-credentials", func(g *ScopedGrant) { g.Operation = NetworkAccess; g.Resource = "https://user:password@example.com" }},
		{"network-path", func(g *ScopedGrant) { g.Operation = NetworkAccess; g.Resource = "https://example.com/private" }},
		{"network-query", func(g *ScopedGrant) { g.Operation = NetworkAccess; g.Resource = "https://example.com?scope=all" }},
		{"network-scheme", func(g *ScopedGrant) { g.Operation = NetworkAccess; g.Resource = "file:///etc/passwd" }},
		{"mcp-missing-tool", func(g *ScopedGrant) { g.Operation = MCPCall; g.Resource = `["server",""]` }},
		{"mcp-extra-field", func(g *ScopedGrant) { g.Operation = MCPCall; g.Resource = `["server","tool","extra"]` }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, ctx := scopedFixture(t)
			valid := grantFixture(ctx)
			if err := p.Grant(valid); err != nil {
				t.Fatal(err)
			}
			bad := valid
			bad.ID = "new"
			bad.Scope = ExactScope
			tc.mutate(&bad)
			if err := p.Grant(bad); err == nil {
				t.Fatal("invalid grant accepted")
			}
			if len(p.Grants()) != 1 || !p.Allowed(ctx, AccessRequest{FileWrite, "file.txt"}) {
				t.Fatal("rejection altered existing authority")
			}
		})
	}
}

func TestPolicyRejectsInvalidWorkspaceAndDigest(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, workspace, digest string }{
		{"missing-workspace", filepath.Join(dir, "missing"), strings.Repeat("a", 64)},
		{"file-workspace", file, strings.Repeat("a", 64)},
		{"short-digest", dir, "abc"},
		{"nonhex-digest", dir, strings.Repeat("z", 64)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := NewScopedPolicy(tc.workspace, tc.digest)
			if p != nil || err == nil {
				t.Fatal("invalid authority context accepted")
			}
		})
	}
}
