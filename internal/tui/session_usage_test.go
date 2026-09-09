package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/harness/llm"
)

func TestResumeRestoresPersistedUsage(t *testing.T) {
	m, manager, oldID := sessionUIFixture(t)
	old, err := manager.Open(context.Background(), oldID, false)
	if err != nil {
		t.Fatal(err)
	}
	r := llm.RequestUsage{ID: "one", Model: "test", Status: "completed", Category: llm.CallGeneration, Source: "reported", Usage: &llm.Usage{InputTokens: 90, OutputTokens: 10}}
	if err := sessionio.RecordRequestUsage(old.Session, r); err != nil {
		t.Fatal(err)
	}
	r.ID = "two"
	r.Source = "unavailable"
	r.Usage = nil
	if err := sessionio.RecordRequestUsage(old.Session, r); err != nil {
		t.Fatal(err)
	}
	old.Session.Close()
	m.Update(m.handleCommand("/resume " + oldID)())
	if m.sessionUsage.InputTokens != 90 || m.sessionUsage.OutputTokens != 10 || m.usageRequests != 2 || m.usageUnknown != 1 {
		t.Fatal("saved usage not restored")
	}
	m.runUsageCommand()
	if !strings.Contains(strings.Join(m.transcript, "\n"), "1 attempts have unknown usage") {
		t.Fatal("unknown attempts not disclosed")
	}
}

func TestCancelledApplicationRestoresTerminalUsageWithoutLateText(t *testing.T) {
	started := make(chan struct{})
	service := app.New(applicationBackend{run: func(ctx context.Context, _ string) (<-chan app.BackendEvent, error) {
		events := make(chan app.BackendEvent, 2)
		go func() {
			close(started)
			<-ctx.Done()
			events <- app.BackendEvent{Text: "late model output"}
			events <- app.BackendEvent{Kind: "session_usage", Details: app.Details{UsageKnown: true, InputTokens: 75, OutputTokens: 5, UsageRequests: 2, UsageUnknown: 1, UsagePriorUnknown: true}}
			close(events)
		}()
		return events, nil
	}}, app.Options{SessionID: "session", MaxIterations: 1})
	m := applicationModel(t, service, t.TempDir())
	m.running = true
	cmd := m.startApplicationGoal("go", nil)
	<-started
	m.goalCancelled = true
	m.cancel()
	driveApplication(t, m, cmd)
	if m.running || m.sessionUsage.InputTokens != 75 || m.usageRequests != 2 || m.usageUnknown != 1 || !m.usagePriorUnknown {
		t.Fatal("cancelled usage not restored")
	}
	if strings.Contains(strings.Join(m.transcript, "\n"), "late model output") {
		t.Fatal("late text rendered after cancellation")
	}
}

func TestUsageDisplaysUnmeasuredHistoryWithoutRecordedAttempts(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.usagePriorUnknown = true
	m.runUsageCommand()
	if !strings.Contains(strings.Join(m.transcript, "\n"), "Earlier session usage is unknown") {
		t.Fatal("unmeasured history hidden")
	}
}
