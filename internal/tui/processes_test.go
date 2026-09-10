//go:build unix

package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/process"
)

func processModel(t *testing.T) *Model {
	t.Helper()
	store, err := process.NewArtifactStore(filepath.Join(t.TempDir(), "capture"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := app.NewProcesses(context.Background(), t.TempDir(), store)
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(nil, t.TempDir())
	m.controller = &Controller{Processes: p}
	t.Cleanup(m.CloseApplication)
	return m
}
func finishProcessTestCommand(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("missing asynchronous command")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		m.Update(msg)
	case <-time.After(5 * time.Second):
		t.Fatal("process operation hung")
	}
}
func TestProcessCommandsDuringForegroundRun(t *testing.T) {
	m := processModel(t)
	m.running = true
	finishProcessTestCommand(t, m, m.handleCommand("/process start read line; printf 'reply:%s' \"$line\""))
	list := m.controller.Processes.List()
	if len(list) != 1 {
		t.Fatalf("list: %+v", list)
	}
	id := list[0].ID
	finishProcessTestCommand(t, m, m.handleCommand("/process send "+id+" hello"))
	finishProcessTestCommand(t, m, m.handleCommand("/process wait "+id))
	m.handleCommand("/process read " + id)
	if !strings.Contains(strings.Join(m.transcript, "\n"), "reply:hello") {
		t.Fatal("captured output absent")
	}
	if !m.running {
		t.Fatal("process controls changed foreground state")
	}
	m.handleCommand("/process forget " + id)
	if len(m.controller.Processes.List()) != 0 {
		t.Fatal("forget failed")
	}
}
func TestProcessWaitAllowsCancellationAndShutdown(t *testing.T) {
	m := processModel(t)
	finishProcessTestCommand(t, m, m.handleCommand("/process start sleep 30"))
	id := m.controller.Processes.List()[0].ID
	wait := m.handleCommand("/process wait " + id)
	m.handleCommand("/process cancel " + id)
	finishProcessTestCommand(t, m, wait)
	finishProcessTestCommand(t, m, m.handleCommand("/process start sleep 30"))
	id = m.controller.Processes.List()[1].ID
	late := m.handleCommand("/process wait " + id)
	m.CloseApplication()
	if m.processTask != nil {
		t.Fatal("unjoined command")
	}
	for _, p := range m.controller.Processes.List() {
		if p.Running {
			t.Fatal("surviving process")
		}
	}
	// A completion delivered after shutdown cannot append stale UI state.
	before := len(m.transcript)
	finishProcessTestCommand(t, m, late)
	if len(m.transcript) != before {
		t.Fatal("stale completion changed transcript")
	}
}
