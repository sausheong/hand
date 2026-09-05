package main

import (
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/permissions"
)

func TestPromptWorkspaceTrust(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"lowercase y", "y\n", true},
		{"lowercase yes", "yes\n", true},
		{"uppercase Y", "Y\n", true},
		{"mixed case Yes", "Yes\n", true},
		{"n", "n\n", false},
		{"empty line", "\n", false},
		{"garbage", "sure\n", false},
		{"EOF with no newline", "y", true},
		{"EOF with nothing read", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := promptWorkspaceTrust(strings.NewReader(tc.input), "/tmp/.hand/settings.json", []string{"bash"})
			if err != nil {
				t.Fatalf("promptWorkspaceTrust returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("promptWorkspaceTrust(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestLoadTrustedPermissions_NoAlwaysAllowEntries_NoPrompt: a workspace
// whose settings.json has no always_allow entries needs no trust
// decision at all — reading from stdin would block forever in a real
// terminal, so an empty stdin reader must never be touched.
func TestLoadTrustedPermissions_NoAlwaysAllowEntries_NoPrompt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace := t.TempDir()

	st, err := loadTrustedPermissions(workspace, strings.NewReader(""))
	if err != nil {
		t.Fatalf("loadTrustedPermissions returned error: %v", err)
	}
	if st.IsAlwaysAllowed("bash") {
		t.Fatal("no always_allow entries were configured, want none allowed")
	}
}

// TestLoadTrustedPermissions_AlreadyTrusted_NoPrompt: once a workspace is
// recorded in the trust file, its settings.json entries are honored
// silently on every subsequent run.
func TestLoadTrustedPermissions_AlreadyTrusted_NoPrompt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace := t.TempDir()

	if err := permissions.Save(permissions.DefaultPath(workspace), permissions.Settings{AlwaysAllow: []string{"bash"}}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	trustPath, err := permissions.TrustPath()
	if err != nil {
		t.Fatalf("TrustPath returned error: %v", err)
	}
	if err := permissions.MarkTrusted(trustPath, workspace); err != nil {
		t.Fatalf("MarkTrusted returned error: %v", err)
	}

	// An empty reader would fail closed if read from — proves no prompt happened.
	st, err := loadTrustedPermissions(workspace, strings.NewReader(""))
	if err != nil {
		t.Fatalf("loadTrustedPermissions returned error: %v", err)
	}
	if !st.IsAlwaysAllowed("bash") {
		t.Fatal("already-trusted workspace's always_allow entries should be honored without prompting")
	}
}

// TestLoadTrustedPermissions_NotYetTrusted_Accept: accepting the prompt
// persists the trust decision and returns a store honoring settings.json.
func TestLoadTrustedPermissions_NotYetTrusted_Accept(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace := t.TempDir()

	if err := permissions.Save(permissions.DefaultPath(workspace), permissions.Settings{AlwaysAllow: []string{"bash"}}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	st, err := loadTrustedPermissions(workspace, strings.NewReader("y\n"))
	if err != nil {
		t.Fatalf("loadTrustedPermissions returned error: %v", err)
	}
	if !st.IsAlwaysAllowed("bash") {
		t.Fatal("accepting the trust prompt should honor settings.json's always_allow entries")
	}

	trustPath, err := permissions.TrustPath()
	if err != nil {
		t.Fatalf("TrustPath returned error: %v", err)
	}
	trust, err := permissions.LoadTrust(trustPath)
	if err != nil {
		t.Fatalf("LoadTrust returned error: %v", err)
	}
	if !trust.IsTrusted(workspace) {
		t.Fatal("accepting the trust prompt should persist the decision to the trust file")
	}
}

// TestLoadTrustedPermissions_NotYetTrusted_Decline: declining the prompt
// returns a store with no always-allow entries for this run, and leaves
// the trust file untouched (asked again next time).
func TestLoadTrustedPermissions_NotYetTrusted_Decline(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace := t.TempDir()

	if err := permissions.Save(permissions.DefaultPath(workspace), permissions.Settings{AlwaysAllow: []string{"bash"}}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	st, err := loadTrustedPermissions(workspace, strings.NewReader("n\n"))
	if err != nil {
		t.Fatalf("loadTrustedPermissions returned error: %v", err)
	}
	if st.IsAlwaysAllowed("bash") {
		t.Fatal("declining the trust prompt must not honor settings.json's always_allow entries")
	}

	trustPath, err := permissions.TrustPath()
	if err != nil {
		t.Fatalf("TrustPath returned error: %v", err)
	}
	trust, err := permissions.LoadTrust(trustPath)
	if err != nil {
		t.Fatalf("LoadTrust returned error: %v", err)
	}
	if trust.IsTrusted(workspace) {
		t.Fatal("declining the trust prompt must not persist the workspace as trusted")
	}
}

// TestLoadTrustedPermissions_NotYetTrusted_DeclineOnReadError: a stdin
// read failure (EOF with nothing read, e.g. non-interactive stdin) must
// fail closed the same as an explicit decline, not error out or hang.
func TestLoadTrustedPermissions_NotYetTrusted_DeclineOnReadError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace := t.TempDir()

	if err := permissions.Save(permissions.DefaultPath(workspace), permissions.Settings{AlwaysAllow: []string{"bash"}}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	st, err := loadTrustedPermissions(workspace, strings.NewReader(""))
	if err != nil {
		t.Fatalf("loadTrustedPermissions returned error: %v", err)
	}
	if st.IsAlwaysAllowed("bash") {
		t.Fatal("a stdin read failure should fail closed (not trusted), not honor always_allow")
	}
}
