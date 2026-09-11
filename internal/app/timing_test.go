package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptrace"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/llm"
)

func TestTimingMilestonesAndPrivacy(t *testing.T) {
	r := newTiming(1)
	ctx := context.WithValue(context.Background(), timingKey{}, r)
	req := llm.ChatRequest{Messages: []llm.Message{{Content: "PRIVATE-PROMPT"}}}
	events, err := timeProviderCall(ctx, req, "stream", func(ctx context.Context, _ llm.ChatRequest) (<-chan llm.ChatEvent, error) {
		trace := httptrace.ContextClientTrace(ctx)
		trace.GotConn(httptrace.GotConnInfo{Reused: true})
		trace.WroteRequest(httptrace.WroteRequestInfo{})
		trace.GotFirstResponseByte()
		ch := make(chan llm.ChatEvent, 3)
		ch <- llm.ChatEvent{Type: llm.EventTextDelta, Text: "PRIVATE-RESPONSE"}
		ch <- llm.ChatEvent{Type: llm.EventToolCallStart}
		ch <- llm.ChatEvent{Type: llm.EventDone}
		close(ch)
		return ch, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for range events {
		count++
	}
	q := r.snapshot().Requests[0]
	if count != 3 || q.Status != "completed" || q.FirstTextMS == nil || q.FirstToolMS == nil || q.ResponseHeadersMS == nil || !*q.ConnectionReused {
		t.Fatalf("%+v events=%d", q, count)
	}
	if *q.FirstTextMS < *q.ResponseHeadersMS || q.DurationMS < *q.FirstToolMS {
		t.Fatal("milestones out of order")
	}
	data, _ := json.Marshal(r.snapshot())
	if strings.Contains(string(data), "PRIVATE") {
		t.Fatal("content leaked")
	}
	// Returned snapshots must not share pointer fields with the recorder.
	*q.FirstTextMS = -1
	if *r.snapshot().Requests[0].FirstTextMS < 0 {
		t.Fatal("snapshot aliases recorder")
	}
}

func TestTimingFailuresAndCancellation(t *testing.T) {
	for _, scenario := range []string{"error", "closed", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			r := newTiming(1)
			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), timingKey{}, r))
			defer cancel()
			failure := errors.New("sentinel")
			stream, err := timeProviderCall(ctx, llm.ChatRequest{}, "stream", func(context.Context, llm.ChatRequest) (<-chan llm.ChatEvent, error) {
				if scenario == "error" {
					return nil, failure
				}
				ch := make(chan llm.ChatEvent)
				if scenario == "closed" {
					close(ch)
				}
				return ch, nil
			})
			if scenario == "error" {
				if err != failure {
					t.Fatal(err)
				}
			} else {
				if scenario == "cancel" {
					cancel()
				}
				select {
				case _, ok := <-stream:
					if ok {
						t.Fatal("unexpected event")
					}
				case <-time.After(time.Second):
					t.Fatal("stream failed to close")
				}
			}
			q := r.snapshot().Requests[0]
			if q.Status == "completed" || q.Status == "running" || q.FirstTextMS != nil {
				t.Fatalf("%+v", q)
			}
		})
	}
}

func TestTimingBoundedConcurrentSnapshots(t *testing.T) {
	r := newTiming(1)
	ctx := context.WithValue(context.Background(), timingKey{}, r)
	var wg sync.WaitGroup
	for i := 0; i < 150; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stream, _ := timeProviderCall(ctx, llm.ChatRequest{}, "stream", func(context.Context, llm.ChatRequest) (<-chan llm.ChatEvent, error) {
				ch := make(chan llm.ChatEvent, 1)
				ch <- llm.ChatEvent{Type: llm.EventDone}
				close(ch)
				return ch, nil
			})
			for range stream {
			}
			_ = r.snapshot()
		}()
	}
	wg.Wait()
	report := r.snapshot()
	if len(report.Requests) != 128 || report.DroppedRequests != 22 {
		t.Fatalf("requests=%d omitted=%d", len(report.Requests), report.DroppedRequests)
	}
}

type timingBackend struct {
	*backendFixture
	saved   TimingReport
	saveErr error
}

func (b *timingBackend) RecordTiming(r TimingReport) error { b.saved = r; return b.saveErr }
func TestServiceTimingBeforeTerminalAndSaveFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		backend := &timingBackend{backendFixture: &backendFixture{run: func(ctx context.Context, _ string) (<-chan BackendEvent, error) {
			if timingFrom(ctx) == nil {
				t.Fatal("no recorder in backend context")
			}
			return completedStream(), nil
		}}}
		if fail {
			backend.saveErr = errors.New("disk unavailable")
		}
		s := New(backend, Options{MaxIterations: 1, Check: func(context.Context, string, int) agentio.GoalLoopOutcome {
			return agentio.GoalLoopOutcome{Verified: true}
		}})
		warnings := 0
		outcome, err := s.Execute(context.Background(), "hi", nil, func(e Event) {
			if e.Kind == "warning" {
				warnings++
			}
			if e.Kind == "terminal" && (!s.Timing().Complete || !backend.saved.Complete) {
				t.Error("timing absent at terminal")
			}
		})
		if err != nil || outcome.Status != agentio.Completed || !backend.saved.Complete || backend.saved.TotalMS <= 0 || backend.saved.ChecksMS < 0 {
			t.Fatalf("outcome=%+v timing=%+v err=%v", outcome, backend.saved, err)
		}
		if fail && warnings != 1 {
			t.Fatal("save failure was hidden")
		}
	}
}

type timingBaseProvider struct{ llm.LLMProvider }
type timingCacheProvider struct{ timingBaseProvider }

func (timingCacheProvider) SupportsPromptCaching() bool { return true }

type timingFallbackProvider struct{ timingBaseProvider }

func (timingFallbackProvider) ChatNonStreaming(context.Context, llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	return nil, errors.New("fallback called")
}

type timingAllProvider struct{ timingFallbackProvider }

func (timingAllProvider) SupportsPromptCaching() bool { return false }
func TestTimingPreservesProviderCapabilities(t *testing.T) {
	for _, p := range []llm.LLMProvider{timingBaseProvider{}, timingCacheProvider{}, timingFallbackProvider{}, timingAllProvider{}} {
		wrapped := withProviderTiming(p)
		_, wantCache := p.(llm.PromptCachingProvider)
		cache, gotCache := wrapped.(llm.PromptCachingProvider)
		_, wantFallback := p.(llm.NonStreamingProvider)
		fallback, gotFallback := wrapped.(llm.NonStreamingProvider)
		if wantCache != gotCache || wantFallback != gotFallback {
			t.Fatalf("changed capabilities for %T", p)
		}
		if gotCache && cache.SupportsPromptCaching() != p.(llm.PromptCachingProvider).SupportsPromptCaching() {
			t.Fatal("changed cache result")
		}
		if gotFallback {
			_, err := fallback.ChatNonStreaming(context.Background(), llm.ChatRequest{})
			if err == nil || err.Error() != "fallback called" {
				t.Fatal("fallback not forwarded")
			}
		}
	}
}

func TestTimingToolAndApprovalSpans(t *testing.T) {
	r := newTiming(1)
	r.observe(Event{Kind: "tool_call", State: Running, Details: Details{ToolID: "a", ToolName: "bash"}})
	r.observe(Event{Kind: "approval_required", State: AwaitingApproval})
	r.observe(Event{Kind: "approval_resolved", State: Running})
	r.observe(Event{Kind: "tool_result", State: Running, Details: Details{ToolID: "a", ToolName: "bash"}})
	report := r.snapshot()
	if len(report.ToolSpans) != 1 || report.ToolSpans[0].Name != "bash" || report.ToolSpans[0].DurationMS <= 0 || report.ApprovalMS <= 0 {
		t.Fatalf("%+v", report)
	}
}

func TestTimingFinalizationSettlesPendingRequest(t *testing.T) {
	r := newTiming(1)
	r.report.Requests = []RequestTiming{{Status: "running"}}
	r.finish(true)
	report := r.snapshot()
	if !report.Complete || report.Requests[0].Status != "cancelled" || report.Requests[0].DurationMS <= 0 {
		t.Fatalf("%+v", report)
	}
}
