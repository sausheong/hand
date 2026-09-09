package tui

import (
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/checkpoints"
	"strings"
	"testing"
)

func TestRecoveryDisplayResolutionScope(t *testing.T) {
	page := app.CheckpointRecoveryPage{Total: 33, Next: 32, Recoveries: []checkpoints.RestoreRecovery{
		{State: "prepared", Event: checkpoints.RestoreEvent{RecoveryName: "pending", Path: "one"}},
		{State: "applied_unrecorded", Event: checkpoints.RestoreEvent{RecoveryName: "unrecorded", Path: "two"}},
		{State: "conflict", Reason: "later edit", Event: checkpoints.RestoreEvent{RecoveryName: "conflict", Path: "three"}},
	}}
	text := strings.Join(recoveryLines(page), "\n")
	for _, want := range []string{"/recovery-resolve cancel pending", "/recovery-resolve acknowledge unrecorded", "later edit", "/recoveries 32", "preserves target"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
	if strings.Contains(text, "cancel conflict") {
		t.Fatal("conflict offered resolution")
	}
}
func TestRecoveryConfirmationConsumesDisplayedRecord(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	page := &app.CheckpointRecoveryPage{Recoveries: []checkpoints.RestoreRecovery{{State: "prepared", Event: checkpoints.RestoreEvent{RecoveryName: "pending"}}}}
	m.recoveryPage = page
	if cmd := m.runRecoveryResolve([]string{"cancel", "unknown"}); cmd != nil {
		t.Fatal("unknown record dispatched")
	}
	if cmd := m.runRecoveryResolve([]string{"acknowledge", "pending"}); cmd != nil {
		t.Fatal("wrong resolution dispatched")
	}
	m.running = true
	if cmd := m.handleCommand("/recovery-resolve cancel pending"); cmd != nil {
		t.Fatal("busy resolution dispatched")
	}
	m.running = false
	cmd := m.runRecoveryResolve([]string{"cancel", "pending"})
	if cmd == nil || m.recoveryPage != nil {
		t.Fatal("review not consumed")
	}
	m.Update(cmd())
	if m.sessionChanging || m.recoveryPage != nil {
		t.Fatal("failed resolution retained review")
	}
	if cmd := m.runRecoveryResolve([]string{"cancel", "pending"}); cmd != nil {
		t.Fatal("resolution replay dispatched")
	}
}
