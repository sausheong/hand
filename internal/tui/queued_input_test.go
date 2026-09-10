package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	"testing"
)

func TestQueuedFollowupTUIEditsAndDispatchesAfterCompletion(t *testing.T) {
	var prompts []string
	backend := applicationBackend{run: func(_ context.Context, p string) (<-chan app.BackendEvent, error) {
		prompts = append(prompts, p)
		ch := make(chan app.BackendEvent, 1)
		ch <- app.BackendEvent{Done: true}
		close(ch)
		return ch, nil
	}}
	m := applicationModel(t, app.New(backend, app.Options{SessionID: "s", MaxIterations: 1}), t.TempDir())
	cmd := m.startRun("initial")
	m.textarea.SetValue("/followup first\nsecond line")
	m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	queue := m.service.QueuedInputs()
	if len(queue) != 1 || queue[0].Text != "first\nsecond line" {
		t.Fatal(queue)
	}
	m.handleCommand("/queue edit " + queue[0].ID + " changed\n  indentation")
	m.handleCommand("/followup remove me")
	queue = m.service.QueuedInputs()
	m.handleCommand("/queue remove " + queue[1].ID)
	driveApplication(t, m, cmd)
	if len(prompts) != 2 || prompts[1] != "changed\n  indentation" {
		t.Fatal(prompts)
	}
	if m.running || len(m.service.QueuedInputs()) != 0 {
		t.Fatal("follow-up did not settle")
	}
}

func TestQueueTypingDoesNotAnswerApproval(t *testing.T) {
	m := applicationModel(t, app.New(nil, app.Options{}), t.TempDir())
	m.running = true
	m.pending = &agentio.ApprovalRequest{Tool: "bash"}
	m.textarea.SetValue("/followup ")
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("say yes later")})
	if m.pending == nil {
		t.Fatal("typing answered approval")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.pending == nil || len(m.service.QueuedInputs()) != 1 {
		t.Fatal("queue submission changed approval")
	}
	m.textarea.SetValue("/queue remove " + m.service.QueuedInputs()[0].ID)
	m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.pending == nil || len(m.service.QueuedInputs()) != 0 {
		t.Fatal("queue removal changed approval")
	}
}

func TestCancelledRunRetainsFollowupForExplicitRun(t *testing.T) {
	calls := 0
	backend := applicationBackend{run: func(ctx context.Context, _ string) (<-chan app.BackendEvent, error) {
		calls++
		ch := make(chan app.BackendEvent, 1)
		if calls == 1 {
			go func() { <-ctx.Done(); close(ch) }()
		} else {
			ch <- app.BackendEvent{Done: true}
			close(ch)
		}
		return ch, nil
	}}
	m := applicationModel(t, app.New(backend, app.Options{MaxIterations: 1}), t.TempDir())
	cmd := m.startRun("cancel me")
	m.handleCommand("/followup keep me")
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	driveApplication(t, m, cmd)
	if len(m.service.QueuedInputs()) != 1 || m.running {
		t.Fatal("cancel lost or started pending input")
	}
	driveApplication(t, m, m.handleCommand("/queue run"))
	if len(m.service.QueuedInputs()) != 0 || calls != 2 {
		t.Fatal("explicit retry did not dispatch once", calls)
	}
}

func TestSteerCommandQueuesCorrectionWithoutApprovingTool(t *testing.T) {
	m := applicationModel(t, app.New(nil, app.Options{}), t.TempDir())
	m.running = true
	m.pending = &agentio.ApprovalRequest{Tool: "bash"}
	m.textarea.SetValue("/steer stop after this tool")
	m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	queue := m.service.QueuedInputs()
	if len(queue) != 1 || queue[0].Queue != app.SteeringQueue || queue[0].Text != "stop after this tool" || m.pending == nil {
		t.Fatal(queue, m.pending)
	}
}
