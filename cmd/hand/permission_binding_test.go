package main

import (
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/hand/internal/permissions"
	"testing"
)

func TestPermissionBindingPreparationAndConfigurationIsolation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Config{}
	p := config.ModelProfile{Provider: "local", Model: "old"}
	a, legacy, digest, err := openCLIAuth(workspace, cfg, p)
	if err != nil {
		t.Fatal(err)
	}
	initial := app.PermissionState{Authority: a, Legacy: legacy, Digest: digest}
	b := &cliPermissionBindings{current: initial, states: map[string]app.PermissionState{digest: initial}, workspace: workspace, cfg: cfg}
	defer b.close()
	proposal, err := permissions.PrepareLegacyMigration(permissions.Settings{AlwaysAllow: []string{"bash"}}, workspace, digest)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.AcknowledgeLegacy(proposal, proposal.Fingerprint()); err != nil {
		t.Fatal(err)
	}
	for _, changed := range []config.ModelProfile{
		{Provider: "local", Model: "new"},
		{Provider: "local", Model: "old", Endpoint: "http://localhost:2345/v1"},
		{Provider: "local", Model: "old", CredentialEnv: "DIFFERENT_KEY"},
	} {
		commit, err := b.prepare(changed)
		if err != nil {
			t.Fatal(err)
		}
		if b.snapshot().Digest != digest {
			t.Fatal("preparation changed live authority")
		}
		commit()
		next := b.snapshot()
		if next.Digest == digest || len(next.Authority.Grants()) != 0 {
			t.Fatal("changed configuration inherited grants")
		}
		back, err := b.prepare(p)
		if err != nil {
			t.Fatal(err)
		}
		back()
		if b.snapshot().Authority != a || len(a.Grants()) != 1 {
			t.Fatal("return to original configuration lost grants")
		}
	}
	b.close()
	if _, err = b.prepare(p); err == nil {
		t.Fatal("closed owner reopened authority")
	}
}
