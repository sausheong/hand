//go:build unix

package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/permissions"
)

func TestPermissionCommandsPersistRevocationAndPage(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "authority")
	digest := strings.Repeat("a", 64)
	a, err := permissions.OpenAuthority(dir, workspace, digest)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for i := 0; i < 18; i++ {
		err = a.Grant(permissions.ScopedGrant{ID: fmt.Sprintf("grant-%02d", i), Operation: permissions.ShellExec, Scope: permissions.ExactScope, Resource: "echo hello", Lifetime: permissions.PersistentGrant, Provenance: permissions.UserDecision, Workspace: workspace, ConfigDigest: digest})
		if err != nil {
			t.Fatal(err)
		}
	}
	m := NewModel(nil, workspace)
	m.controller = &Controller{Authority: a}
	defer m.CloseApplication()
	m.running = true
	finishProcessTestCommand(t, m, m.handleCommand("/permissions"))
	out := strings.Join(m.transcript, "\n")
	if !strings.Contains(out, "Next page: /permissions 16") || strings.Contains(out, "grant-17:") {
		t.Fatal(out)
	}
	finishProcessTestCommand(t, m, m.handleCommand("/permissions 16"))
	if !strings.Contains(strings.Join(m.transcript, "\n"), "grant-17:") {
		t.Fatal("second page absent")
	}
	cmd := m.handleCommand("/permissions revoke grant-17")
	if cmd == nil {
		t.Fatal("revocation was not asynchronous")
	}
	m.CloseApplication() // joins even if Bubble Tea never consumes the result
	m.Update(cmd())      // stale completion after shutdown must be ignored
	if !m.running {
		t.Fatal("permission controls changed foreground state")
	}
	if len(a.Grants()) != 17 {
		t.Fatal("revocation absent")
	}
	a.Close()
	a, err = permissions.OpenAuthority(dir, workspace, digest)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if len(a.Grants()) != 17 {
		t.Fatal("revocation did not survive reopen")
	}
}

func TestPermissionLegacyReviewRequiresExactAcknowledgement(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("b", 64)
	a, err := permissions.OpenAuthority(filepath.Join(t.TempDir(), "authority"), workspace, digest)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	proposal, err := permissions.PrepareLegacyMigration(permissions.Settings{AlwaysAllow: []string{"bash"}}, workspace, digest)
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(nil, workspace)
	m.controller = &Controller{Authority: a, LegacyPermissions: proposal}
	defer m.CloseApplication()
	finishProcessTestCommand(t, m, m.handleCommand("/permissions legacy"))
	out := strings.Join(m.transcript, "\n")
	for _, want := range []string{"broad persistent per-tool", `Tool: "bash"`, workspace, digest, proposal.Fingerprint()} {
		if !strings.Contains(out, want) {
			t.Fatalf("review missing %q: %s", want, out)
		}
	}
	if len(a.Grants()) != 0 {
		t.Fatal("review imported authority")
	}
	finishProcessTestCommand(t, m, m.handleCommand("/permissions acknowledge wrong-fingerprint"))
	if len(a.Grants()) != 0 {
		t.Fatal("wrong fingerprint imported authority")
	}
	finishProcessTestCommand(t, m, m.handleCommand("/permissions acknowledge "+proposal.Fingerprint()))
	grants := a.Grants()
	if len(grants) != 1 || grants[0].Operation != permissions.LegacyTool || grants[0].Provenance != permissions.LegacyAcknowledged {
		t.Fatalf("incorrect migration: %+v", grants)
	}
	finishProcessTestCommand(t, m, m.handleCommand("/permissions acknowledge "+proposal.Fingerprint()))
	if len(a.Grants()) != 1 {
		t.Fatal("repeated acknowledgement duplicated authority")
	}
}
