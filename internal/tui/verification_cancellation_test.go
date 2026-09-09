//go:build darwin || linux

package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/checkpoints"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/process"
)

func verificationCancellationModel(t *testing.T) (*Model, string) {
	t.Helper()
	m, _, _ := sessionUIFixture(t)
	workspace := t.TempDir()
	marker := filepath.Join(t.TempDir(), "pid")
	outputs, err := process.NewArtifactStore(filepath.Join(t.TempDir(), "output"))
	if err != nil {
		t.Fatal(err)
	}
	processes, err := app.NewProcesses(context.Background(), workspace, outputs)
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
	if err = os.Chmod(evidence, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := config.VerificationConfig{Directory: evidence, MaxRecords: 10, MaxBytes: 1 << 20, Profiles: []config.VerificationProfile{{Name: "held", Command: []string{"/bin/sh", "-c", `echo $$ > "$1"; exec sleep 30`, "verify", marker}}}}
	if err = m.controller.ConfigureVerification(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.CloseApplication)
	return m, marker
}

func TestVerificationCancellationJoinsChildLatency(t *testing.T) {
	for _, action := range []string{"ctrl_c", "quit", "exit", "close"} {
		t.Run(action, func(t *testing.T) {
			samples := make([]float64, 0, 30)
			for trial := 0; trial < 30; trial++ {
				m, marker := verificationCancellationModel(t)
				finishProcessTestCommand(t, m, m.handleCommand("/verify held"))
				if len(m.verificationReviews) != 1 {
					t.Fatal("verification review missing")
				}
				cmd := m.handleCommand("/verify-confirm held " + m.verificationReviews[0].Digest)
				if cmd == nil {
					t.Fatal("confirmation did not start")
				}
				result := make(chan tea.Msg, 1)
				go func() { result <- cmd() }()
				deadline := time.Now().Add(3 * time.Second)
				pid := 0
				for time.Now().Before(deadline) {
					data, _ := os.ReadFile(marker)
					pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
					if pid > 0 {
						break
					}
					time.Sleep(time.Millisecond)
				}
				if pid == 0 {
					t.Fatal("verification child did not start")
				}
				if err := syscall.Kill(pid, 0); err != nil {
					t.Fatalf("child not active: %v", err)
				}
				begin := time.Now()
				switch action {
				case "close":
					m.CloseApplication()
				case "quit", "exit":
					if next := m.handleCommand("/" + action); next != nil {
						t.Fatal("quit before worker joined")
					}
				default:
					m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
				}
				var msg tea.Msg
				select {
				case msg = <-result:
				case <-time.After(5 * time.Second):
					t.Fatal("verification did not join")
				}
				samples = append(samples, float64(time.Since(begin).Nanoseconds())/1e6)
				if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
					t.Fatalf("verification result before child reaped: %v", err)
				}
				m.Update(msg)
				if m.sessionChanging {
					t.Fatal("verification retained operation guard")
				}
				if strings.Contains(strings.Join(m.transcript, "\n"), "passed (exit 0)") {
					t.Fatal("cancelled verification labelled passed")
				}
				m.CloseApplication()
			}
			sort.Float64s(samples)
			t.Logf("child_cleanup_ms_sorted=%v p95_ms=%f trials=%d", samples, samples[28], len(samples))
			if samples[29] > 5000 {
				t.Fatalf("child cleanup exceeded 5s: %f", samples[29])
			}
		})
	}
}
