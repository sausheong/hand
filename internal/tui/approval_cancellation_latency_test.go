package tui

import (
	"sort"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/agentio"
)

// Measure a real service approval broker unblocking after the UI cancels.
// Backend cleanup stays held until after the signal observation, so this also
// rejects terminal publication that precedes the backend join.
func TestApprovalCancellationInitiationLatency(t *testing.T) {
	for _, action := range []string{"ctrl_c", "quit", "close", "viewer", "viewer_search"} {
		t.Run(action, func(t *testing.T) {
			samples := make([]float64, 0, 30)
			for trial := 0; trial < 30; trial++ {
				m, _, returned, release := pendingServiceApproval(t)
				stream := m.activeStream
				if action == "viewer" || action == "viewer_search" {
					m.outputView = &outputViewer{searching: action == "viewer_search"}
				}
				closed := make(chan struct{})
				start := time.Now()
				switch action {
				case "close":
					go func() { m.CloseApplication(); close(closed) }()
				case "quit":
					m.handleCommand("/quit")
				default:
					m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
				}
				select {
				case ctx := <-returned:
					samples = append(samples, float64(time.Since(start).Nanoseconds())/1e6)
					if ctx.Err() == nil {
						release()
						t.Fatal("approval returned without cancellation")
					}
				case <-time.After(3 * time.Second):
					release()
					t.Fatal("approval did not unblock")
				}
				select {
				case <-stream.Done:
					release()
					t.Fatal("terminal preceded held backend cleanup")
				default:
				}
				release()
				outcome, err := stream.Wait()
				if err != nil || outcome.Status != agentio.Cancelled {
					t.Fatalf("cancel outcome: %+v %v", outcome, err)
				}
				if action == "close" {
					select {
					case <-closed:
					case <-time.After(3 * time.Second):
						t.Fatal("close did not join")
					}
				} else {
					m.CloseApplication()
				}
			}
			sort.Float64s(samples)
			t.Logf("cancel_start_ms_sorted=%v p95_ms=%f trials=%d", samples, samples[28], len(samples))
			if samples[28] > 1000 {
				t.Fatalf("approval cancellation p95 exceeds limit: %f", samples[28])
			}
		})
	}
}
