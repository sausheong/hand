package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/sausheong/hand/internal/config"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func invocationFixture(t *testing.T, args ...string) {
	t.Helper()
	previousArgs, previousFlags, previousLogger := os.Args, flag.CommandLine, slog.Default()
	t.Cleanup(func() { os.Args = previousArgs; flag.CommandLine = previousFlags; slog.SetDefault(previousLogger) })
	os.Args = append([]string{"hand"}, args...)
	flag.CommandLine = flag.NewFlagSet("hand", flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
}

func TestInvocationRejectsConfigurationBeforeProviderCall(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   []string
		reason string
	}{
		{"positional", []string{"unexpected"}, "positional"},
		{"negative turns", []string{"--max-turns=-1"}, "negative"},
		{"negative iterations", []string{"--max-iterations=-1"}, "negative"},
		{"negative context", []string{"--context-limit=-1"}, "negative"},
		{"unknown reasoning", []string{"--model=local/model", "--reasoning=unknown", "-p", "hello"}, "unsupported"},
		{"undeclared reasoning", []string{"--model=local/model", "--reasoning=high", "-p", "hello"}, "not declared supported"},
		{"approval without prompt", []string{"--yes"}, "only applies"},
		{"invalid provider", []string{"--model=absent/model", "-p", "hello"}, "unknown provider"},
		{"missing credential", []string{"--model=openai/model", "-p", "hello"}, "OPENAI_API_KEY is not set"},
		{"proxy missing URL", []string{"--model=litellm/model", "-p", "hello"}, "requires --base-url"},
		{"unsupported URL", []string{"--model=gemini/model", "--base-url=http://127.0.0.1:1", "-p", "hello"}, "not supported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invocationFixture(t, tc.args...)
			t.Setenv("OPENAI_API_KEY", "")
			err := run()
			if err == nil || exitCode(err) != 2 || !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("want invocation failure %q; got %v", tc.reason, err)
			}
		})
	}
}

func TestInvocationSendsPromptToConfiguredLocalProvider(t *testing.T) {
	type request struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	requests := make(chan request, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "unexpected endpoint", 400)
			return
		}
		var body request
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad JSON", 400)
			return
		}
		requests <- body
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"fixture\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"fixture answer\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"fixture\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	invocationFixture(t, "--model=local/fixture-model", "--base-url="+server.URL+"/v1", "-p", "verify this exact prompt")
	if err := run(); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-requests:
		if body.Model != "fixture-model" {
			t.Fatalf("wrong model %q", body.Model)
		}
		found := false
		for _, message := range body.Messages {
			if message.Role == "user" && strings.Contains(string(message.Content), "verify this exact prompt") {
				found = true
			}
		}
		if !found {
			t.Fatal("configured prompt missing from provider request")
		}
	default:
		t.Fatal("no provider request")
	}
	if len(requests) != 0 {
		t.Fatal("unexpected duplicate provider request")
	}
}

func TestNamedProfileRoutesOnlyItsCredential(t *testing.T) {
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			MaxTokens int `json:"max_completion_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.MaxTokens != 123 {
			t.Errorf("configured output limit missing: %d (%v)", payload.MaxTokens, err)
		}
		requests <- r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"answer\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	invocationFixture(t, "--profile=proxy", "-p", "work")
	t.Setenv("PROFILE_TEST_KEY", "profile-fixture-key")
	t.Setenv("LITELLM_API_KEY", "must-not-cross-over")
	path, err := config.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	c := config.Config{Model: "local/old", BaseURL: "http://127.0.0.1:1", Profiles: map[string]config.ModelProfile{"proxy": {Provider: "litellm", Model: "fixture", Endpoint: server.URL + "/v1", CredentialEnv: "PROFILE_TEST_KEY", MaxOutput: 123}}}
	if err := config.Save(path, c); err != nil {
		t.Fatal(err)
	}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	select {
	case header := <-requests:
		if header != "Bearer profile-fixture-key" {
			t.Fatal("wrong credential routed")
		}
	default:
		t.Fatal("profile endpoint not called")
	}
	if len(requests) != 0 {
		t.Fatal("unexpected duplicate request")
	}
}
