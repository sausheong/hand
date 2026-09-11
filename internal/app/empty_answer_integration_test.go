package app

import (
	"context"
	"fmt"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/providers/openai"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOutputLimitFailsHandTurnAndAppearsInTiming(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"index":0,"delta":{},"finish_reason":"length"}],"usage":{"prompt_tokens":29828,"completion_tokens":2048,"total_tokens":31876}}`+"\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	p := withProviderTiming(openai.NewOpenAIProviderWithKind("fixture", server.URL, "openai-compatible"))
	rt := &runtime.Runtime{LLM: p, Tools: tool.NewRegistry(), Session: session.NewSession("hand", "test"), Model: "fixture", MaxOutputTokens: 2048, MaxTurns: 2}
	reason := ""
	rt.AgentLoop.Hooks.OnStop = func(_ context.Context, r string) { reason = r }
	service := New(&HarnessBackend{Runtime: rt, Reason: &reason}, Options{SessionID: "test", MaxIterations: 1})
	outcome, err := service.Execute(context.Background(), "question", nil, func(Event) {})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status == agentio.Completed || outcome.Cause == nil || !strings.Contains(outcome.Cause.Error(), "output limit") {
		t.Fatalf("%+v", outcome)
	}
	report := service.Timing()
	if len(report.Requests) != 1 {
		t.Fatalf("%+v", report)
	}
	q := report.Requests[0]
	if q.StopReason != "length" || q.MaxOutput != 2048 || q.Status != "output_limit" || q.FirstTextMS != nil {
		t.Fatalf("%+v", q)
	}
}
