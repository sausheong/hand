package tui

import (
	"context"
	"sort"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Measures the production UI/session-operation dispatcher reaching a worker
// context. Filesystem cancellation and crash recovery require separate tests.
func TestSessionOperationCancellationInitiationLatency(t *testing.T) {
	for _, action := range []string{"ctrl_c", "quit", "exit", "close"} {
		t.Run(action, func(t *testing.T) {
			samples := make([]float64, 0, 30)
			for trial := 0; trial < 30; trial++ {
				m, _, _ := sessionUIFixture(t)
				entered := make(chan struct{})
				observed := make(chan time.Time, 1)
				cmd := m.startSessionChange("resume", func(ctx context.Context) error {
					close(entered)
					<-ctx.Done()
					observed <- time.Now()
					return ctx.Err()
				})
				select {
				case <-entered:
				case <-time.After(3 * time.Second):
					m.CloseApplication()
					t.Fatal("session worker did not start")
				}
				identity := m.identity
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
				case signal := <-observed:
					samples = append(samples, float64(signal.Sub(start).Nanoseconds())/1e6)
				case <-time.After(3 * time.Second):
					m.CloseApplication()
					t.Fatal("session cancellation did not reach worker")
				}
				result := cmd().(sessionChangedMsg)
				if result.err != context.Canceled {
					m.CloseApplication()
					t.Fatalf("wrong cancellation cause: %v", result.err)
				}
				_, quit := m.Update(result)
				if action == "quit" || action == "exit" {
					if quit == nil {
						t.Fatal("cancelled session did not finish quit")
					}
					if _, ok := quit().(tea.QuitMsg); !ok {
						t.Fatal("expected actual quit")
					}
				}
				if m.identity != identity || m.sessionChanging {
					t.Fatal("cancelled session changed identity or retained ownership")
				}
				// A queued duplicate may arrive after a fresh operation starts. It must
				// neither release that operation's guard nor install old session data.
				next := m.startSessionOperation("metadata", func(context.Context) sessionChangedMsg {
					return sessionChangedMsg{preserveView: true}
				})
				m.Update(result)
				if !m.sessionChanging || m.identity != identity {
					t.Fatal("stale result disturbed newer operation")
				}
				m.Update(next())
				if m.sessionChanging || m.identity != identity {
					t.Fatal("fresh operation failed after cancellation")
				}
				m.CloseApplication()
			}
			sort.Float64s(samples)
			t.Logf("cancel_start_ms_sorted=%v p95_ms=%f trials=%d", samples, samples[28], len(samples))
			if samples[28] > 1000 {
				t.Fatalf("session cancellation p95 %f ms exceeds 1000 ms", samples[28])
			}
		})
	}
}
