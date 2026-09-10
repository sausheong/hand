package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/hand/protocol"
)

func TestBinaryOutcomeExitCodeMatrix(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "hand")
	if out, err := exec.CommandContext(ctx, "go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	hook := func(event string, code int, policy string) []config.HookConfig {
		return []config.HookConfig{{Event: event, Command: "sh", Args: []string{"-c", fmt.Sprintf("exit %d", code)}, FailurePolicy: policy}}
	}
	for _, tc := range []struct {
		name     string
		hooks    []config.HookConfig
		mode     string
		code     int
		status   agentio.RunStatus
		reason   string
		verified bool
		calls    int32
	}{
		{name: "answer", code: 0, status: agentio.Completed, reason: "answer_completed", calls: 1},
		{name: "verified", hooks: hook("Stop", 0, ""), code: 0, status: agentio.Completed, reason: "answer_completed", verified: true, calls: 1},
		{name: "optional_warning", hooks: hook("Stop", 1, "warn"), code: 0, status: agentio.Completed, reason: "answer_completed", calls: 1},
		{name: "stop_failure", hooks: hook("Stop", 1, ""), code: 3, status: agentio.VerificationFailed, reason: "stop_validator_failed", calls: 1},
		{name: "prompt_failure", hooks: hook("UserPromptSubmit", 2, ""), code: 3, status: agentio.VerificationFailed, reason: "prompt_validator_failed"},
		{name: "iteration_limit", hooks: hook("Stop", 2, ""), code: 4, status: agentio.BudgetExhausted, reason: "max_iterations", calls: 1},
		{name: "turn_limit", mode: "tool", code: 4, status: agentio.BudgetExhausted, reason: "max_turns", calls: 1},
		{name: "provider_failure", mode: "error", code: 5, status: agentio.InfrastructureError, reason: "runtime_error", calls: 1},
		{name: "invalid_invocation", mode: "invalid", code: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if _, err := io.Copy(io.Discard, r.Body); err != nil {
					return
				}
				if tc.mode == "error" {
					http.Error(w, `{"error":{"message":"fixture rejection","type":"invalid_request_error"}}`, http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				delta := map[string]any{"content": "fixture answer"}
				finish := "stop"
				if tc.mode == "tool" {
					delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "missing", "type": "function", "function": map[string]any{"name": "absent", "arguments": "{}"}}}}
					finish = "tool_calls"
				}
				for _, event := range []any{map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}}, map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}}}} {
					raw, _ := json.Marshal(event)
					fmt.Fprintf(w, "data: %s\n\n", raw)
				}
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer server.Close()
			home := t.TempDir()
			if err := os.MkdirAll(filepath.Join(home, ".hand"), 0700); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(config.Config{Hooks: tc.hooks})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, ".hand", "config.json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
			flags := []string{"--model=local/fixture", "--base-url=" + server.URL + "/v1", "--jsonl", "--max-turns=1", "--max-iterations=1", "-p", "exercise outcome"}
			if tc.mode == "invalid" {
				flags = append(flags, "--max-turns=-1")
			}
			cmd := exec.CommandContext(ctx, binary, flags...)
			cmd.Dir = t.TempDir()
			cmd.Env = append(os.Environ(), "HOME="+home)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err = cmd.Run()
			code := 0
			if err != nil {
				var exit *exec.ExitError
				if !errors.As(err, &exit) {
					t.Fatal(err)
				}
				code = exit.ExitCode()
			}
			if code != tc.code || calls.Load() != tc.calls {
				t.Fatalf("code=%d calls=%d want %d/%d stderr=%s stdout=%s", code, calls.Load(), tc.code, tc.calls, stderr.String(), stdout.String())
			}
			reader := protocol.NewReader(&stdout)
			terminals := 0
			var sequence uint64
			for {
				frame, err := reader.ReadFrame()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				var event protocol.Event
				if err := json.Unmarshal(frame, &event); err != nil {
					t.Fatal(err)
				}
				if terminals > 0 || event.Sequence <= sequence {
					t.Fatalf("unordered or post-terminal event %+v", event)
				}
				sequence = event.Sequence
				if event.Kind == "terminal" {
					terminals++
					var outcome agentio.RunOutcome
					if err := json.Unmarshal(event.Payload, &outcome); err != nil {
						t.Fatal(err)
					}
					if outcome.Status != tc.status || outcome.Reason != tc.reason || outcome.Verified != tc.verified {
						t.Fatalf("outcome %+v", outcome)
					}
				}
			}
			want := 1
			if tc.mode == "invalid" {
				want = 0
			}
			if terminals != want {
				t.Fatalf("terminals=%d want=%d", terminals, want)
			}
		})
	}
}
