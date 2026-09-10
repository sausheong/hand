package permissions

import (
	"strings"
	"testing"
)

func TestScopedOperationLifetimeMatrix(t *testing.T) {
	operations := []struct {
		op              Operation
		resource, other string
	}{
		{FileRead, "file.txt", "other.txt"},
		{FileWrite, "file.txt", "other.txt"},
		{SkillWrite, "skill.md", "other.md"},
		{CommandExec, `["go","test","./..."]`, `["go","test","./other"]`},
		{ShellExec, "go test ./...", "go test ./...; touch other"},
		{NetworkAccess, "https://example.com", "https://other.example.com"},
		{MCPCall, `["server","tool"]`, `["other","tool"]`},
		{ToolCall, "tool-digest", "different-digest"},
	}
	for _, operation := range operations {
		for _, lifetime := range []Lifetime{InvocationGrant, SessionGrant, PersistentGrant} {
			t.Run(string(operation.op)+"/"+string(lifetime), func(t *testing.T) {
				policy, ctx := scopedFixture(t)
				g := grantFixture(ctx)
				g.Operation = operation.op
				g.Resource = operation.resource
				g.Scope = ExactScope
				g.Lifetime = lifetime
				if lifetime != InvocationGrant {
					g.InvocationID = ""
				}
				if lifetime == PersistentGrant {
					g.SessionID = ""
				}
				if err := policy.Grant(g); err != nil {
					t.Fatal(err)
				}
				same := ctx
				invocation := ctx
				invocation.InvocationID = "next"
				session := ctx
				session.SessionID = "next"
				anonymous := ctx
				anonymous.SessionID = ""
				anonymous.InvocationID = ""
				config := ctx
				config.ConfigDigest = strings.Repeat("b", 64)
				workspace := ctx
				workspace.Workspace = t.TempDir()
				for _, tc := range []struct {
					name    string
					context AuthorityContext
					allowed bool
				}{
					{"same", same, true},
					{"new-invocation", invocation, lifetime != InvocationGrant},
					{"new-session", session, lifetime == PersistentGrant},
					{"missing-identities", anonymous, lifetime == PersistentGrant},
					{"changed-config", config, false},
					{"changed-workspace", workspace, false},
				} {
					if got := policy.Allowed(tc.context, AccessRequest{operation.op, operation.resource}); got != tc.allowed {
						t.Fatalf("%s allowed=%t want %t", tc.name, got, tc.allowed)
					}
				}
				if policy.Allowed(ctx, AccessRequest{operation.op, operation.other}) {
					t.Fatal("resource scope expanded")
				}
				otherOp := FileWrite
				if operation.op == FileWrite {
					otherOp = FileRead
				}
				if policy.Allowed(ctx, AccessRequest{otherOp, operation.resource}) {
					t.Fatal("operation scope expanded")
				}
				if !policy.Revoke(g.ID) || policy.Allowed(ctx, AccessRequest{operation.op, operation.resource}) {
					t.Fatal("revocation ineffective")
				}
			})
		}
	}
}
