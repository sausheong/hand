package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/compaction"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
)

func TestProfileSwitchHTTPGenerationAndSummaryStayOnSelectedRoute(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	t.Setenv("PROFILE_HOSTED_KEY", "hosted-fixture")
	t.Setenv("PROFILE_PROXY_KEY", "proxy-fixture")
	t.Setenv("OPENAI_API_KEY", "ambient-must-not-cross")
	t.Setenv("LITELLM_API_KEY", "ambient-proxy-must-not-cross")
	type request struct {
		endpoint, authorization, model string
		output                         int
	}
	records := make(chan request, 12)
	profiles := map[string]config.ModelProfile{}
	for _, tc := range []struct{ name, provider, key string }{{"local", "local", ""}, {"hosted", "openai", "PROFILE_HOSTED_KEY"}, {"proxy", "litellm", "PROFILE_PROXY_KEY"}} {
		tc := tc
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var payload struct {
				Model               string `json:"model"`
				MaxCompletionTokens int    `json:"max_completion_tokens"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, "invalid JSON", 400)
				return
			}
			records <- request{tc.name, r.Header.Get("Authorization"), payload.Model, payload.MaxCompletionTokens}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"fixture response\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}))
		defer server.Close()
		profiles[tc.name] = config.ModelProfile{Provider: tc.provider, Model: tc.name + "-model", Endpoint: server.URL + "/v1", CredentialEnv: tc.key, ContextLimit: 32768, MaxOutput: 123}
	}
	rt := &runtime.Runtime{Tools: tool.NewRegistry(), Session: session.NewSession("hand", "test"), Model: "initial", MaxTurns: 1, Compaction: &compaction.Manager{Summarizer: &compaction.Summarizer{MaxOutputTokens: 512}}}
	c := &Controller{Rt: rt, BuildProfileProvider: func(p config.ModelProfile) (llm.LLMProvider, error) { return BuildProfileProvider(ctx, p) }}
	if err := c.ConfigureProfiles(profiles, ""); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"local", "hosted", "proxy", "local"} {
		if err := c.SwitchProfileContext(ctx, name); err != nil {
			t.Fatal(err)
		}
		events, err := rt.Run(ctx, "answer", nil)
		if err != nil {
			t.Fatal(err)
		}
		for event := range events {
			if event.Error != nil {
				t.Fatal(event.Error)
			}
		}
		text, err := rt.Compaction.Summarizer.Summarize(ctx, []session.SessionEntry{session.UserMessageEntry("remember this")}, "")
		if err != nil || text != "fixture response" {
			t.Fatal("summary failed", text, err)
		}
		wantKey := map[string]string{"local": "", "hosted": "Bearer hosted-fixture", "proxy": "Bearer proxy-fixture"}[name]
		for _, output := range []int{123, 512} {
			select {
			case got := <-records:
				if got.endpoint != name || got.authorization != wantKey || got.model != name+"-model" || got.output != output {
					t.Fatalf("route %s output %d: %+v", name, output, got)
				}
			case <-ctx.Done():
				t.Fatal("missing generation or summary request")
			}
		}
	}
	if len(records) != 0 {
		t.Fatal("unexpected requests")
	}
}
