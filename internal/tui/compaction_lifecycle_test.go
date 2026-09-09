package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sausheong/harness/compaction"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

type waitingSummaryProvider struct {
	llm.LLMProvider
	entered   chan struct{}
	cancelled chan struct{}
}

func (p *waitingSummaryProvider) ChatStream(ctx context.Context, _ llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	close(p.entered)
	<-ctx.Done()
	close(p.cancelled)
	return nil, ctx.Err()
}

func compactionTestModel(t *testing.T) (*Model, *waitingSummaryProvider) {
	t.Helper()
	p := &waitingSummaryProvider{entered: make(chan struct{}), cancelled: make(chan struct{})}
	sess := session.NewSession("hand", "test")
	for i := 0; i < 8; i++ {
		sess.Append(session.UserMessageEntry("question"))
		sess.Append(session.AssistantMessageEntry("answer"))
	}
	rt := &runtime.Runtime{Session: sess, Compaction: &compaction.Manager{PreserveTurns: 1, Summarizer: &compaction.Summarizer{Provider: p, Model: "test", Timeout: 2 * time.Second}}}
	m := NewModel(nil, t.TempDir())
	m.controller = &Controller{Rt: rt, Owner: m.service}
	return m, p
}
func TestManualCompactionReturnsCommandWithoutCallingProvider(t *testing.T) {
	m, p := compactionTestModel(t)
	cmd := m.handleCommand("/compact")
	t.Cleanup(func() {
		if m.compactCancel != nil {
			m.compactCancel()
		}
	})
	if cmd == nil {
		t.Fatal("manual compaction must return an asynchronous command")
	}
	select {
	case <-p.entered:
		t.Fatal("provider ran inside Update")
	default:
	}
}

func TestManualCompactionCancellationPreservesTranscript(t *testing.T) {
	m, p := compactionTestModel(t)
	before := m.controller.Rt.Session.View()
	cmd := m.handleCommand("/compact")
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	select {
	case <-p.entered:
	case <-time.After(time.Second):
		t.Fatal("provider never started")
	}
	// Updates and rendering continue while the provider is blocked.
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !strings.Contains(m.statusLine(), "compacting") {
		t.Fatal("compaction activity not visible")
	}
	m.textarea.SetValue("new work")
	m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m.handleCommand("/new")
	if m.service.Snapshot().RunID != 0 {
		t.Fatal("new run raced compaction")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !m.compacting {
		t.Fatal("released worker before it returned")
	}
	select {
	case <-p.cancelled:
	case <-time.After(time.Second):
		t.Fatal("provider not cancelled")
	}
	select {
	case msg := <-result:
		m.Update(msg)
	case <-time.After(time.Second):
		t.Fatal("worker did not return")
	}
	if m.compacting {
		t.Fatal("compaction still active")
	}
	if !reflect.DeepEqual(before, m.controller.Rt.Session.View()) {
		t.Fatal("cancelled compaction changed transcript")
	}
	if !strings.Contains(strings.Join(m.transcript, "\n"), "compaction cancelled") {
		t.Fatal("missing cancellation result")
	}
}

func TestManualCompactionCancelBeforeDispatchAndStaleResults(t *testing.T) {
	m, p := compactionTestModel(t)
	cmd := m.handleCommand("/compact")
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	old := cmd()
	m.Update(old)
	select {
	case <-p.entered:
		t.Fatal("cancelled queued command invoked provider")
	default:
	}
	next := m.handleCommand("/compact")
	m.Update(old)
	if !m.compacting {
		t.Fatal("stale result released newer operation")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	m.Update(next())
}

func TestManualCompactionQuitWaitsForWorker(t *testing.T) {
	m, _ := compactionTestModel(t)
	work := m.handleCommand("/compact")
	if cmd := m.handleCommand("/quit"); cmd != nil {
		t.Fatal("quit must await compaction shutdown")
	}
	_, cmd := m.Update(work())
	if cmd == nil {
		t.Fatal("quit did not finish after worker stopped")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected quit")
	}
}
