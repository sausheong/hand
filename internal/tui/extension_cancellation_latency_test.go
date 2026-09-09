package tui

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/extensions"
)

// Measure through the joined command result, a conservative upper bound on
// cancellation initiation. The peer is a real compiled extension waiting for
// a question response; no model or network calls are involved.
func TestExtensionCancellationJoinedLatency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "note")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./examples/extensions/go-task-note")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	review, err := extensions.ReviewHostLaunch(ctx, extensions.LaunchConfig{Name: "task-note", Executable: binary,
		Workspace: t.TempDir(), Capabilities: []string{"commands", "questions", "state", "context.transform"}})
	if err != nil {
		t.Fatal(err)
	}
	factory, err := extensions.NewHostFactory(filepath.Join(t.TempDir(), "private"), []extensions.LaunchReview{review}, func(context.Context, extensions.LaunchReview) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"ctrl_c", "quit", "exit", "close", "viewer", "viewer_search"} {
		t.Run(action, func(t *testing.T) {
			samples := make([]float64, 0, 30)
			for trial := 0; trial < 30; trial++ {
				func() {
					m, _, _ := sessionUIFixture(t)
					defer m.controller.Rt.Session.Close()
					defer m.CloseApplication()
					host, err := app.NewExtensionHost(ctx, m.controller, factory, map[string]string{"task-note": "examples/note"})
					if err != nil {
						t.Fatal(err)
					}
					defer host.Close()
					m.controller.Extensions = host
					if _, err := host.Reload(ctx, []extensions.Specification{review.Specification}); err != nil {
						t.Fatal(err)
					}
					m.textarea.SetValue("preserve this draft")
					command := m.runExtension([]string{"task-note", "note"})
					if command == nil {
						t.Fatal("extension operation not started")
					}
					batch := command().(tea.BatchMsg)
					deadline := time.Now().Add(3 * time.Second)
					for len(host.Pending()) == 0 && time.Now().Before(deadline) {
						time.Sleep(time.Millisecond)
					}
					m.pollExtensionQuestion(extensionQuestionTick{m.extensionGeneration})
					if m.extensionQuestion == nil || !m.sessionChanging {
						t.Fatal("real peer question did not hold operation")
					}
					if action == "viewer" || action == "viewer_search" {
						m.outputView = &outputViewer{searching: action == "viewer_search"}
					}
					result := make(chan tea.Msg, 1)
					go func() { result <- batch[0]() }()
					closed := make(chan struct{})
					start := time.Now()
					switch action {
					case "close":
						go func() { m.CloseApplication(); close(closed) }()
					case "quit":
						m.handleCommand("/quit")
					case "exit":
						m.handleCommand("/exit")
					default:
						m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
					}
					var message tea.Msg
					select {
					case message = <-result:
					case <-time.After(5 * time.Second):
						t.Fatal("extension cancellation did not join")
					}
					if action == "close" {
						select {
						case <-closed:
						case <-time.After(5 * time.Second):
							t.Fatal("application close did not join")
						}
					}
					samples = append(samples, float64(time.Since(start).Nanoseconds())/1e6)
					changed, ok := message.(sessionChangedMsg)
					if !ok || !errors.Is(changed.err, context.Canceled) {
						t.Fatalf("cancelled command result: %#v", message)
					}
					if len(host.Pending()) != 0 {
						t.Fatal("cancelled question retained")
					}
					m.Update(message)
					if m.sessionChanging {
						t.Fatal("joined command retained ownership")
					}
					if m.extensionQuestion != nil || m.textarea.Value() != "preserve this draft" {
						t.Fatal("cancelled question did not restore the draft")
					}
					state, err := extensions.NewStateStore(m.controller.Rt.Session, "examples/note")
					if err != nil {
						t.Fatal(err)
					}
					snapshot, err := state.Get(ctx)
					if err != nil || snapshot.Revision != 0 {
						t.Fatalf("cancelled question changed durable state: %+v %v", snapshot, err)
					}
				}()
			}
			sort.Float64s(samples)
			t.Logf("extension_join_ms_sorted=%v p95_ms=%f trials=%d", samples, samples[28], len(samples))
			if samples[28] > 1000 || samples[29] > 5000 {
				t.Fatal("extension cancellation exceeded latency or cleanup deadline")
			}
		})
	}
}
