//go:build unix

package permissions

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectBashPersistenceAndBoundaries(t *testing.T) {
	workspace, _ := filepath.EvalSymlinks(t.TempDir())
	dir := filepath.Join(t.TempDir(), "authority")
	digest := strings.Repeat("a", 64)
	a, err := OpenAuthority(dir, workspace, digest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { a.Close() }()
	g, err := a.AllowProjectBash()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.AllowProjectBash(); err != nil || len(a.Grants()) != 1 {
		t.Fatal("grant not idempotent", err)
	}
	a.Close()
	a, err = OpenAuthority(dir, workspace, digest)
	if err != nil {
		t.Fatal(err)
	}
	ctx := AuthorityContext{Workspace: workspace, ConfigDigest: digest}
	for _, cmd := range []string{"pwd", "go test ./...", "echo a && echo b"} {
		if !a.AllowedTool(ctx, "bash", AccessRequest{Operation: ShellExec, Resource: cmd}) {
			t.Fatal("command denied", cmd)
		}
	}
	if a.AllowedTool(ctx, "write_file", AccessRequest{Operation: FileWrite, Resource: filepath.Join(workspace, "x")}) {
		t.Fatal("broadened unrelated tool")
	}
	other := ctx
	other.ConfigDigest = strings.Repeat("b", 64)
	if a.AllowedTool(other, "bash", AccessRequest{Operation: ShellExec, Resource: "pwd"}) {
		t.Fatal("cross-config grant")
	}
	other = ctx
	other.Workspace = t.TempDir()
	if a.AllowedTool(other, "bash", AccessRequest{Operation: ShellExec, Resource: "pwd"}) {
		t.Fatal("cross-project grant")
	}
	if err = a.Revoke(g.ID); err != nil {
		t.Fatal(err)
	}
	if a.AllowedTool(ctx, "bash", AccessRequest{Operation: ShellExec, Resource: "pwd"}) {
		t.Fatal("revocation ineffective")
	}
	a.Close()
	a, err = OpenAuthority(dir, workspace, digest)
	if err != nil {
		t.Fatal(err)
	}
	if a.AllowedTool(ctx, "bash", AccessRequest{Operation: ShellExec, Resource: "pwd"}) {
		t.Fatal("revocation not durable")
	}
	if _, err = a.AllowProjectBash(); err != nil {
		t.Fatal("regrant", err)
	}
}

func TestAllScopeRejectsInvalidGrants(t *testing.T) {
	workspace, _ := filepath.EvalSymlinks(t.TempDir())
	p, err := NewScopedPolicy(workspace, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	base := ScopedGrant{ID: "test", Operation: ShellExec, Scope: AllScope, Resource: "*", Lifetime: PersistentGrant, Provenance: UserDecision, Workspace: workspace, ConfigDigest: strings.Repeat("a", 64)}
	for _, op := range []Operation{FileRead, FileWrite, CommandExec, NetworkAccess, MCPCall, ToolCall, LegacyTool} {
		g := base
		g.Operation = op
		if p.Grant(g) == nil {
			t.Fatal("invalid all scope accepted", op)
		}
	}
	g := base
	g.Resource = "pwd"
	if p.Grant(g) == nil {
		t.Fatal("non-wildcard accepted")
	}
	g = base
	g.Provenance = LegacyAcknowledged
	if p.Grant(g) == nil {
		t.Fatal("legacy provenance accepted")
	}
}
