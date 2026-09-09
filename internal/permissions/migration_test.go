//go:build darwin || linux

package permissions

import "testing"

func TestLegacyMigrationRequiresExactAcknowledgement(t *testing.T) {
	_, ctx := scopedFixture(t)
	a, err := OpenAuthority(authorityFixtureDir(t), ctx.Workspace, ctx.ConfigDigest)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	proposal, err := PrepareLegacyMigration(Settings{AlwaysAllow: []string{"bash", "write_file", "bash"}}, ctx.Workspace, ctx.ConfigDigest)
	if err != nil {
		t.Fatal(err)
	}
	request := AccessRequest{ShellExec, "go test ./...; arbitrary child code"}
	if a.AllowedTool(ctx, "bash", request) {
		t.Fatal("proposal became authority")
	}
	if err = a.AcknowledgeLegacy(proposal, "unreviewed"); err == nil {
		t.Fatal("missing acknowledgement accepted")
	}
	if err = a.AcknowledgeLegacy(proposal, proposal.Fingerprint()); err != nil {
		t.Fatal(err)
	}
	if !a.AllowedTool(ctx, "bash", request) {
		t.Fatal("acknowledged broad meaning silently narrowed")
	}
	if a.AllowedTool(ctx, "other_tool", request) {
		t.Fatal("legacy tool scope expanded")
	}
	if err = a.AcknowledgeLegacy(proposal, proposal.Fingerprint()); err != nil || len(a.Grants()) != 2 {
		t.Fatal("migration not resumable", err)
	}
	changed, _ := PrepareLegacyMigration(Settings{AlwaysAllow: []string{"bash", "write_file", "process"}}, ctx.Workspace, ctx.ConfigDigest)
	if changed.Fingerprint() == proposal.Fingerprint() {
		t.Fatal("changed grants reuse acknowledgement")
	}
	if err = a.AcknowledgeLegacy(changed, proposal.Fingerprint()); err == nil {
		t.Fatal("changed scope accepted")
	}
	grants := proposal.Grants()
	grants[0].Resource = "other_tool"
	if proposal.Grants()[0].Resource == "other_tool" {
		t.Fatal("mutable proposal")
	}
}
func TestBroadToolGrantCannotUseOrdinaryProvenance(t *testing.T) {
	p, ctx := scopedFixture(t)
	g := persistentFixture(ctx)
	g.Operation = LegacyTool
	g.Scope = ExactScope
	g.Resource = "bash"
	if err := p.Grant(g); err == nil {
		t.Fatal("broad tool grant bypassed legacy acknowledgement")
	}
}
