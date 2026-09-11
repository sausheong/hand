package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
)

func TestTimingRealHTTPStreamingAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(25 * time.Millisecond)
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\n")
		w.(http.Flusher).Flush()
		time.Sleep(25 * time.Millisecond)
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":1,\"total_tokens\":11}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	p, err := BuildProfileProvider(context.Background(), config.ModelProfile{Provider: "litellm", Endpoint: server.URL + "/v1"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		r := newTiming(uint64(i + 1))
		ctx := context.WithValue(context.Background(), timingKey{}, r)
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		observed, settled := 0, 0
		ctx = llm.WithUsageObserver(ctx, func(llm.RequestUsage) { observed++ })
		ctx = llm.WithCallAdmission(ctx, func(context.Context, llm.ChatRequest, llm.CallCategory, string) (func(llm.RequestUsage) error, error) {
			return func(llm.RequestUsage) error { settled++; return nil }, nil
		})
		stream, err := llm.ObserveChat(ctx, llm.ChatRequest{Model: "fixture", Messages: []llm.Message{{Role: "user", Content: "hi"}}}, llm.CallGeneration, p.ChatStream)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		for event := range stream {
			if event.Error != nil {
				t.Error(event.Error)
			}
		}
		cancel()
		q := r.snapshot().Requests[0]
		if q.Usage == nil || q.Usage.InputTokens != 10 || observed != 1 || settled != 1 {
			t.Fatalf("usage=%+v observers=%d settlements=%d", q, observed, settled)
		}
		if q.FirstTextMS == nil || q.ResponseHeadersMS == nil || *q.FirstTextMS-*q.ResponseHeadersMS < 20 || q.DurationMS-*q.FirstTextMS < 20 {
			t.Fatalf("did not distinguish header/text/stream delays: %+v", q)
		}
		if i == 1 && (q.ConnectionReused == nil || !*q.ConnectionReused) {
			t.Fatal("expected reused HTTP connection")
		}
	}
	// Exercise the actual service/runtime path, not just the provider adapter.
	sess := session.NewSession("hand", "test")
	rt := &runtime.Runtime{LLM: p, Tools: tool.NewRegistry(), Session: sess, Model: "fixture", MaxTurns: 1}
	svc := NewHarness(rt, nil, nil, t.TempDir(), 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	outcome, err := svc.Execute(ctx, "hi", nil, nil)
	if err != nil || outcome.Status != agentio.Completed {
		t.Fatalf("%+v %v", outcome, err)
	}
	report := svc.Timing()
	if !report.Complete || len(report.Requests) != 1 || report.Requests[0].FirstTextMS == nil || report.Requests[0].Status != "completed" {
		t.Fatalf("%+v", report)
	}
	if len(sess.Annotations("hand.timing")) != 1 {
		t.Fatal("service did not persist timings")
	}
}

func TestTimingSessionAnnotation(t *testing.T) {
	sess := session.NewSession("hand", "test")
	b := &HarnessBackend{}
	// RecordTiming uses the same annotation channel as the session run journal.
	b.Runtime = &runtime.Runtime{Session: sess}
	report := newTiming(42).snapshot()
	report.Complete = true
	if err := b.RecordTiming(report); err != nil {
		t.Fatal(err)
	}
	records := sess.Annotations("hand.timing")
	if len(records) != 1 {
		t.Fatal(len(records))
	}
	var saved TimingReport
	if err := json.Unmarshal(records[0].Payload, &saved); err != nil || saved.RunID != 42 || !saved.Complete {
		t.Fatalf("%+v %v", saved, err)
	}
}
