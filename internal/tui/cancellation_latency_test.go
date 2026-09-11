package tui

import (
	"context"
	"github.com/sausheong/harness/llm"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
)

// Measure delivery through the production Model and Service to the context
// observed by an active backend/checker. This does not measure provider/network
// cancellation or descendant cleanup, which require separate native journeys.
func TestApplicationCancellationInitiationLatency(t *testing.T) {
	for _, phase := range []string{"backend", "checker"} {
		for _, action := range []string{"ctrl_c", "close", "viewer", "viewer_search"} {
			t.Run(phase+"/"+action, func(t *testing.T) {
				samples := make([]float64, 0, 30)
				for trial := 0; trial < 30; trial++ {
					samples = append(samples, measureCancellationSignal(t, phase, action))
				}
				sort.Float64s(samples)
				p95 := samples[28] // nearest-rank ceil(30 * 0.95)
				t.Logf("cancel_start_ms_sorted=%v p95_ms=%f trials=%d", samples, p95, len(samples))
				if p95 > 1000 {
					t.Fatalf("cancellation initiation p95 %f ms exceeds 1000 ms", p95)
				}
			})
		}
	}
}

func measureCancellationSignal(t *testing.T, phase, action string) float64 {
	t.Helper()
	started := make(chan struct{})
	observed := make(chan time.Time, 1)
	wait := func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		observed <- time.Now()
	}
	backend := applicationBackend{run: func(ctx context.Context, _ string) (<-chan app.BackendEvent, error) {
		ch := make(chan app.BackendEvent, 1)
		if phase == "backend" {
			go func() { wait(ctx); close(ch) }()
		} else {
			ch <- app.BackendEvent{Done: true}
			close(ch)
		}
		return ch, nil
	}}
	options := app.Options{SessionID: "latency", MaxIterations: 1}
	if phase == "checker" {
		options.Check = func(ctx context.Context, _ string, _ int) agentio.GoalLoopOutcome {
			wait(ctx)
			return agentio.GoalLoopOutcome{}
		}
	}
	m := applicationModel(t, app.New(backend, options), t.TempDir())
	m.startRun("cancellation latency")
	defer m.CloseApplication()
	stream := m.activeStream
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("backend/checker did not start")
	}
	start := time.Now()
	if action == "viewer" || action == "viewer_search" || action == "escape_viewer" {
		m.outputView = &outputViewer{searching: action == "viewer_search"}
	}
	if action == "escape_approval" {
		m.pending = &agentio.ApprovalRequest{Tool: "bash"}
	}
	if strings.HasPrefix(action, "escape") {
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	} else if action != "close" {
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	} else {
		m.CloseApplication()
	}
	var signal time.Time
	select {
	case signal = <-observed:
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not reach backend/checker context")
	}
	select {
	case <-stream.Done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled backend/checker did not join")
	}
	outcome, err := stream.Wait()
	if err != nil || outcome.Status != agentio.Cancelled {
		t.Fatalf("cancellation outcome=%+v err=%v", outcome, err)
	}
	return float64(signal.Sub(start).Nanoseconds()) / 1e6
}

func TestEscapeCancelsActiveTurn(t *testing.T) {
	for _, phase := range []string{"backend", "checker"} {
		for _, action := range []string{"escape", "escape_viewer", "escape_approval"} {
			t.Run(phase+"/"+action, func(t *testing.T) { measureCancellationSignal(t, phase, action) })
		}
	}
}

func TestEscapeIdleDoesNotQuitAndCancelsCompaction(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	defer m.CloseApplication()
	m.textarea.SetValue("keep draft")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || m.textarea.Value() != "keep draft" {
		t.Fatal("idle Esc quit or cleared ordinary draft")
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.compacting = true
	m.compactCancel = cancel
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if ctx.Err() == nil || !m.compactCancelled {
		t.Fatal("Esc did not cancel compaction")
	}
	m.compacting = false
}

func TestOutputLoadCancellationInitiationLatency(t *testing.T) {
	for _, action := range []string{"escape", "ctrl_c", "close"} {
		t.Run(action, func(t *testing.T) {
			samples := make([]float64, 0, 30)
			for trial := 0; trial < 30; trial++ {
				m := NewModel(nil, t.TempDir())
				viewer := &outputViewer{}
				m.outputView = viewer
				started := make(chan struct{})
				observed := make(chan time.Time, 1)
				cmd := m.startOutputLoad(viewer, func(ctx context.Context) (ToolOutput, error) {
					close(started)
					<-ctx.Done()
					observed <- time.Now()
					// A reader can return data concurrently with cancellation;
					// it must not restore a closed or subsequently replaced viewer.
					return ToolOutput{Output: "late cancelled output"}, nil
				})
				<-started
				start := time.Now()
				switch action {
				case "escape":
					m.Update(tea.KeyMsg{Type: tea.KeyEsc})
				case "ctrl_c":
					m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
				case "close":
					m.CloseApplication()
				}
				select {
				case signal := <-observed:
					samples = append(samples, float64(signal.Sub(start).Nanoseconds())/1e6)
				case <-time.After(3 * time.Second):
					viewer.load.cancel()
					m.CloseApplication()
					t.Fatal("viewer cancellation did not reach reader")
				}
				m.CloseApplication()
				replacement := &outputViewer{block: ToolOutput{Output: "new output"}}
				m.outputView = replacement
				m.Update(cmd())
				if len(m.outputLoads) != 0 || m.outputView != replacement || replacement.block.Output != "new output" {
					t.Fatal("cancelled reader retained or late result replaced current output")
				}
			}
			sort.Float64s(samples)
			t.Logf("cancel_start_ms_sorted=%v p95_ms=%f trials=%d", samples, samples[28], len(samples))
			if samples[28] > 1000 {
				t.Fatalf("output cancellation p95 %f ms exceeds 1000 ms", samples[28])
			}
		})
	}
}

// The provider timestamps context observation itself, so worker completion and
// result delivery do not inflate or replace the cancellation signal interval.
type timedCompactionProvider struct {
	llm.LLMProvider
	entered  chan struct{}
	observed chan time.Time
}

func (p *timedCompactionProvider) ChatStream(ctx context.Context, _ llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	close(p.entered)
	<-ctx.Done()
	p.observed <- time.Now()
	return nil, ctx.Err()
}

func TestCompactionCancellationInitiationLatency(t *testing.T) {
	for _, action := range []string{"ctrl_c", "quit", "exit", "close"} {
		t.Run(action, func(t *testing.T) {
			samples := make([]float64, 0, 30)
			for trial := 0; trial < 30; trial++ {
				m, _ := compactionTestModel(t)
				provider := &timedCompactionProvider{entered: make(chan struct{}), observed: make(chan time.Time, 1)}
				m.controller.Rt.Compaction.Summarizer.Provider = provider
				// This timeout is a test watchdog, not the intended cancellation source.
				m.controller.Rt.Compaction.Summarizer.Timeout = 10 * time.Second
				before := m.controller.Rt.Session.View()
				cmd := m.handleCommand("/compact")
				result := make(chan tea.Msg, 1)
				go func() { result <- cmd() }()
				select {
				case <-provider.entered:
				case <-time.After(3 * time.Second):
					m.CloseApplication()
					t.Fatal("compaction provider did not start")
				}
				start := time.Now()
				switch action {
				case "ctrl_c":
					m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
				case "quit", "exit":
					m.textarea.SetValue("/" + action)
					m.Update(tea.KeyMsg{Type: tea.KeyEnter})
				case "close":
					m.CloseApplication()
				}
				select {
				case signal := <-provider.observed:
					samples = append(samples, float64(signal.Sub(start).Nanoseconds())/1e6)
				case <-time.After(3 * time.Second):
					m.CloseApplication()
					t.Fatal("compaction cancellation did not reach provider")
				}
				select {
				case msg := <-result:
					m.Update(msg)
				case <-time.After(3 * time.Second):
					m.CloseApplication()
					t.Fatal("compaction worker did not return")
				}
				m.CloseApplication()
				if m.controller.Rt.Compaction.HasInFlight(m.controller.Rt.Session) {
					t.Fatal("compaction remained in flight")
				}
				if !reflect.DeepEqual(before, m.controller.Rt.Session.View()) {
					t.Fatal("cancelled compaction changed transcript")
				}
			}
			sort.Float64s(samples)
			t.Logf("cancel_start_ms_sorted=%v p95_ms=%f trials=%d", samples, samples[28], len(samples))
			if samples[28] > 1000 {
				t.Fatalf("compaction cancellation p95 %f ms exceeds 1000 ms", samples[28])
			}
		})
	}
}
