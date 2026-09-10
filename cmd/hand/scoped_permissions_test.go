package main

import (
	"encoding/json"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/hand/internal/permissions"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIAuthLegacyReviewImportAndChangedConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, ".hand"), 0700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(permissions.Settings{AlwaysAllow: []string{"bash"}})
	if err := os.WriteFile(permissions.DefaultPath(workspace), raw, 0600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{}
	profile := config.ModelProfile{Provider: "local", Model: "test"}
	a, proposal, digest, err := openCLIAuth(workspace, cfg, profile)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Grants()) != 0 || len(proposal.Grants()) != 1 {
		t.Fatal("legacy file self-authorised")
	}
	if err = a.AcknowledgeLegacy(proposal, proposal.Fingerprint()); err != nil {
		t.Fatal(err)
	}
	a.Close()
	a, _, againDigest, err := openCLIAuth(workspace, cfg, profile)
	if err != nil {
		t.Fatal(err)
	}
	if againDigest != digest || len(a.Grants()) != 1 {
		t.Fatal("acknowledged grant lost")
	}
	a.Close()
	profile.Endpoint = "http://127.0.0.1:1234/v1"
	a, changed, newDigest, err := openCLIAuth(workspace, cfg, profile)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if newDigest == digest || len(a.Grants()) != 0 || changed.Fingerprint() == proposal.Fingerprint() {
		t.Fatal("changed config reused approval")
	}
}
func TestPermissionsInspectionDoesNotRequireCredentials(t *testing.T) {
	invocationFixture(t, "--permissions", "--model=openai/test")
	t.Setenv("OPENAI_API_KEY", "")
	if err := run(); err != nil {
		t.Fatal(err)
	}
}

func TestCLIAuthCorruptionIdentifiesRecoveryDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspace := t.TempDir()
	cfg := config.Config{}
	profile := config.ModelProfile{Provider: "local", Model: "test"}
	a, _, _, err := openCLIAuth(workspace, cfg, profile)
	if err != nil {
		t.Fatal(err)
	}
	a.Close()
	paths, err := filepath.Glob(filepath.Join(home, ".hand", "authority", "*", "*", "grants.jsonl"))
	if err != nil || len(paths) != 1 {
		t.Fatal(paths, err)
	}
	bad := []byte("{corrupt}\n")
	if err = os.WriteFile(paths[0], bad, 0600); err != nil {
		t.Fatal(err)
	}
	a, _, _, err = openCLIAuth(workspace, cfg, profile)
	if a != nil {
		a.Close()
	}
	if err == nil || !strings.Contains(err.Error(), filepath.Dir(paths[0])) {
		t.Fatal("recovery location missing", err)
	}
	got, err := os.ReadFile(paths[0])
	if err != nil || string(got) != string(bad) {
		t.Fatal("evidence modified", err)
	}
}
