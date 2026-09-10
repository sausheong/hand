package main

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/harness/runtime"
)

func TestRuntimeStartupVisibleAndCancellationJoined(t *testing.T) {
	started := make(chan struct{})
	joined := make(chan struct{})
	s := newRuntimeStartup(func(ctx context.Context) (*runtime.Runtime, error) {
		close(started)
		<-ctx.Done()
		close(joined)
		return nil, ctx.Err()
	}, 2)
	if !strings.Contains(s.View(), "Connecting 2") {
		t.Fatal("startup state hidden")
	}
	cmd := s.Init()
	<-started
	s.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !strings.Contains(s.View(), "cancelling") {
		t.Fatal("cancellation state hidden")
	}
	s.Update(cmd())
	rt, err := s.finish(nil)
	if rt != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("finish: %v %v", rt, err)
	}
	select {
	case <-joined:
	default:
		t.Fatal("construction not joined")
	}
}
func TestRuntimeStartupDisconnectBeforeInit(t *testing.T) {
	s := newRuntimeStartup(func(ctx context.Context) (*runtime.Runtime, error) { return nil, ctx.Err() }, 1)
	disconnected := errors.New("terminal disconnected")
	rt, err := s.finish(disconnected)
	if rt != nil || !errors.Is(err, disconnected) || !errors.Is(err, context.Canceled) {
		t.Fatalf("disconnect: %v %v", rt, err)
	}
}
func TestRuntimeStartupSuccessTransfersRuntime(t *testing.T) {
	want := &runtime.Runtime{}
	s := newRuntimeStartup(func(context.Context) (*runtime.Runtime, error) { return want, nil }, 1)
	cmd := s.Init()
	s.Update(cmd())
	got, err := s.finish(nil)
	if got != want || err != nil {
		t.Fatalf("runtime transfer: %v %v", got, err)
	}
}

func TestRuntimeStartupDraftSurvivesFailureAndRetry(t *testing.T) {
	attempts := 0
	want := &runtime.Runtime{}
	s := newRuntimeStartup(func(context.Context) (*runtime.Runtime, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("required server unavailable")
		}
		return want, nil
	}, 1)
	cmd := s.Init()
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("query draft")})
	old := cmd()
	_, quit := s.Update(old)
	if quit != nil || !s.failed || s.draft.Value() != "query draft" {
		t.Fatal("failure lost draft or exited")
	}
	_, retry := s.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if retry == nil {
		t.Fatal("retry not started")
	}
	// A prior attempt's queued completion cannot settle the new attempt.
	if _, cmd := s.Update(old); cmd != nil || s.failed {
		t.Fatal("stale completion applied")
	}
	s.Update(retry())
	got, err := s.finish(nil)
	if got != want || err != nil || attempts != 2 || s.draft.Value() != "query draft" {
		t.Fatalf("retry: %v %v draft=%q", got, err, s.draft.Value())
	}
}

// Measures the production startup UI action reaching the app-owned builder
// context. Real MCP process cleanup is covered by separate integration tests.
func TestRuntimeStartupCancellationInitiationLatency(t *testing.T) {
	for _, action := range []string{"ctrl_c", "escape", "disconnect"} {
		t.Run(action, func(t *testing.T) {
			samples := make([]float64, 0, 30)
			for i := 0; i < 30; i++ {
				started := make(chan struct{})
				observed := make(chan time.Time, 1)
				s := newRuntimeStartup(func(ctx context.Context) (*runtime.Runtime, error) {
					close(started)
					<-ctx.Done()
					observed <- time.Now()
					return nil, ctx.Err()
				}, 1)
				cmd := s.Init()
				<-started
				begin := time.Now()
				if action == "disconnect" {
					_, _ = s.finish(errors.New("terminal disconnected"))
				} else {
					key := tea.KeyCtrlC
					if action == "escape" {
						key = tea.KeyEsc
					}
					s.Update(tea.KeyMsg{Type: key})
				}
				select {
				case at := <-observed:
					samples = append(samples, float64(at.Sub(begin).Nanoseconds())/1e6)
				case <-time.After(3 * time.Second):
					t.Fatal("startup cancellation not delivered")
				}
				if action != "disconnect" {
					s.Update(cmd())
					rt, err := s.finish(nil)
					if rt != nil || !errors.Is(err, context.Canceled) {
						t.Fatalf("cancel result: %v %v", rt, err)
					}
				}
			}
			sort.Float64s(samples)
			t.Logf("cancel_start_ms_sorted=%v p95_ms=%f trials=%d", samples, samples[28], len(samples))
			if samples[28] > 1000 {
				t.Fatalf("startup cancellation p95 exceeded limit: %f", samples[28])
			}
		})
	}
}
