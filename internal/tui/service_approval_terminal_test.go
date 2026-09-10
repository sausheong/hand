package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	teatest "github.com/charmbracelet/x/exp/teatest"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/permissions"
)

func terminalApprovalJourney(t *testing.T, name, input, preview string, key tea.KeyMsg, wantAllow, wantAlways bool) {
	t.Helper()
	workspace := t.TempDir()
	storePath := filepath.Join(t.TempDir(), "permissions.json")
	store, err := permissions.NewStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	hook := app.NewApprovalHook(store, workspace, nil, nil)
	allowed := make(chan bool, 1)
	backend := applicationBackend{run: func(ctx context.Context, _ string) (<-chan app.BackendEvent, error) {
		events := make(chan app.BackendEvent, 1)
		go func() {
			decision, err := hook(ctx, name, []byte(input))
			allowed <- decision.Allow
			events <- app.BackendEvent{Done: true, Err: err}
			close(events)
		}()
		return events, nil
	}}
	m := applicationModel(t, app.New(backend, app.Options{SessionID: "approval-terminal", MaxIterations: 1}), workspace)
	defer m.CloseApplication()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	m.BindProgram(tm.GetProgram())
	tm.Type("request the operation")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	teatest.WaitFor(t, tm.Output(), func(output []byte) bool {
		return strings.Contains(string(output), "Allow "+name+"?") && strings.Contains(string(output), preview)
	}, teatest.WithDuration(2*time.Second))
	select {
	case <-allowed:
		t.Fatal("operation passed the broker before user decision")
	default:
	}
	tm.Send(key)
	select {
	case actual := <-allowed:
		if actual != wantAllow {
			t.Fatalf("allow=%v, want %v", actual, wantAllow)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("service approval did not resolve")
	}
	teatest.WaitFor(t, tm.Output(), func(output []byte) bool { return strings.Contains(string(output), "[result] Completed") }, teatest.WithDuration(2*time.Second))
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
	if m.pending != nil || m.pendingApprovalID != "" || m.service.Snapshot().State != app.Idle || m.lastOutcome == nil || m.lastOutcome.Status != agentio.Completed {
		t.Fatal("approval lifetime did not settle", m.lastOutcome)
	}
	text := strings.Join(m.transcript, "\n")
	if wantAllow && !strings.Contains(text, map[bool]string{false: "approved: ", true: "always allowed: "}[wantAlways]+name) {
		t.Fatal("approval result missing", text)
	}
	if strings.Count(text, "[result]") != 1 {
		t.Fatal("duplicate terminal result")
	}
	reopened, err := permissions.NewStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.IsAlwaysAllowed(name) != wantAlways {
		t.Fatal("always decision not preserved exactly")
	}
}

func TestModel_ApprovalPromptBlocksAndRespondsYes(t *testing.T) {
	terminalApprovalJourney(t, "write_file", `{"path":"x.txt"}`, "", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}, true, false)
}
func TestModel_ApprovalPromptShowsPreview(t *testing.T) {
	terminalApprovalJourney(t, "bash", `{"command":"go test ./..."}`, "$ go test ./...", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}, false, false)
}
func TestModel_ApprovalPromptRespondsNoOnAnyOtherKey(t *testing.T) {
	terminalApprovalJourney(t, "bash", `{"command":"rm -rf /"}`, "", tea.KeyMsg{Type: tea.KeyEnter}, false, false)
}
func TestModel_ApprovalPromptRespondsAlwaysOnA(t *testing.T) {
	terminalApprovalJourney(t, "write_file", `{"path":"x.txt"}`, "[y]es / [a]lways / [n]o", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}, true, true)
}
