//go:build unix

package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/checkpoints"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/process"
)

// This exercises the command dispatcher and asynchronous result handling with
// real process, checkpoint and evidence stores. It is not a PTY rendering test.
func TestVerificationCommandJourney(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	ctx := context.Background()
	workspace := t.TempDir()
	marker := filepath.Join(t.TempDir(), "executions")
	outputs, err := process.NewArtifactStore(filepath.Join(t.TempDir(), "output"))
	if err != nil {
		t.Fatal(err)
	}
	processes, err := app.NewProcesses(ctx, workspace, outputs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { processes.Close() })
	store, err := checkpoints.OpenStore(filepath.Join(t.TempDir(), "checkpoints"), workspace, checkpoints.DefaultStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	boundary := &app.WorkspaceCheckpoints{Workspace: workspace, Limits: checkpoints.DefaultLimits(), Store: store, Processes: processes}
	m.controller.Owner = app.New(&app.HarnessBackend{Runtime: m.controller.Rt}, app.Options{RunBoundary: boundary})
	m.controller.Processes, m.controller.OutputStore = processes, outputs
	evidence := t.TempDir()
	if err := os.Chmod(evidence, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := config.VerificationConfig{Directory: evidence, MaxRecords: 10, MaxBytes: 1 << 20, Profiles: []config.VerificationProfile{{Name: "unit", Command: []string{"/bin/sh", "-c", `printf x >> "$1"; printf verified`, "verify", marker}}}}
	if err := m.controller.ConfigureVerification(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	identity := m.identity
	dispatch := func(command string) {
		t.Helper()
		finishProcessTestCommand(t, m, m.handleCommand(command))
		if m.sessionChanging || m.identity != identity {
			t.Fatal("operation retained worker or changed session")
		}
	}
	text := func() string { return strings.Join(m.transcript, "\n") }
	dispatch("/verify unit")
	if len(m.verificationReviews) != 1 || !strings.Contains(text(), "review before confirming") {
		t.Fatal("missing command review", text())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("review executed command", err)
	}
	digest := m.verificationReviews[0].Digest
	if cmd := m.handleCommand("/verify-confirm unit wrong"); cmd != nil {
		t.Fatal("wrong confirmation dispatched")
	}
	dispatch("/verify-confirm unit " + digest)
	if data, err := os.ReadFile(marker); err != nil || string(data) != "x" {
		t.Fatal("command did not execute exactly once", string(data), err)
	}
	if !strings.Contains(text(), "passed (exit 0)") || !strings.Contains(text(), "stdout: verified") {
		t.Fatal("missing successful result", text())
	}
	if cmd := m.handleCommand("/verify-confirm unit " + digest); cmd != nil {
		t.Fatal("confirmation replay dispatched")
	}
	dispatch("/verify-list")
	if len(m.evidenceIDs) != 1 {
		t.Fatal("saved evidence not listed", text())
	}
	id := m.evidenceIDs[0]
	dispatch("/verify-check unit " + id)
	if err := os.WriteFile(filepath.Join(workspace, "later-edit"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	dispatch("/verify-check unit " + id)
	if !strings.Contains(text(), "stale (exit 0)") {
		t.Fatal("old successful result presented as current", text())
	}
	if cmd := m.handleCommand("/verify-delete unlisted confirm"); cmd != nil {
		t.Fatal("unlisted deletion dispatched")
	}
	// Re-list because intervening commands may invalidate review state.
	dispatch("/verify-list")
	dispatch("/verify-delete " + id + " confirm")
	if _, err := os.Stat(filepath.Join(evidence, id+".json")); !os.IsNotExist(err) {
		t.Fatal("record was not deleted", err)
	}
	dispatch("/verify-list")
	if len(m.evidenceIDs) != 0 {
		t.Fatal("deleted evidence still listed")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "x" {
		t.Fatal("inspection or deletion reran command", string(data), err)
	}
}

func TestVerificationCommandsRejectUnavailableAndInvalidRequests(t *testing.T) {
	for _, command := range []string{"/verify", "/verify missing", "/verify-check unit missing", "/verify-list", "/recoveries"} {
		t.Run(command, func(t *testing.T) {
			m, _, _ := sessionUIFixture(t)
			before := m.identity
			finishProcessTestCommand(t, m, m.handleCommand(command))
			if m.sessionChanging || m.identity != before {
				t.Fatal("failed inspection retained ownership or changed session")
			}
			if !strings.Contains(strings.Join(m.transcript, "\n"), "checkpoint capture is not configured") {
				t.Fatal("missing configuration failure", m.transcript)
			}
		})
	}
	for _, command := range []string{"/verify one two", "/verify-check", "/verify-check unit", "/verify-list -1", "/verify-list invalid", "/verify-list 1 2", "/recoveries -1", "/recoveries invalid", "/recoveries 1 2"} {
		t.Run(command, func(t *testing.T) {
			m, _, _ := sessionUIFixture(t)
			if cmd := m.handleCommand(command); cmd != nil || m.sessionChanging {
				t.Fatal("invalid request dispatched")
			}
			if !strings.Contains(strings.Join(m.transcript, "\n"), "Usage:") {
				t.Fatal("missing usage diagnostic", m.transcript)
			}
		})
	}
}
