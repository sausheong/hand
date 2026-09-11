package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/runtime"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestSessionAutoApprovalCoversEveryToolAndHonoursCancellation(t *testing.T) {
	policy := &app.SessionApproval{}
	policy.SetSkip(true)
	approve := policy.Wrap(func(context.Context, string, json.RawMessage) (runtime.HookDecision, error) {
		return runtime.HookDecision{Allow: false, Reason: "ask"}, nil
	})
	for _, name := range []string{"bash", "write_file", "edit_file", "read_file", "mcp__server__tool", "future_extension_tool"} {
		decision, err := approve(context.Background(), name, nil)
		if err != nil || !decision.Allow {
			t.Fatalf("%s: %+v %v", name, decision, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	decision, err := approve(ctx, "bash", nil)
	if decision.Allow || !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled request was approved")
	}
	policy.SetSkip(false)
	decision, err = approve(context.Background(), "bash", nil)
	if err != nil || decision.Allow || decision.Reason != "ask" {
		t.Fatal("normal hook was not restored")
	}
}

func TestSessionAutoApprovalFlagDoesNotPersist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		finished := false
		for _, m := range body.Messages {
			if m.Role == "tool" {
				finished = true
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if finished {
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		args, _ := json.Marshal(map[string]string{"path": "approved.txt", "content": "approved"})
		payload := map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "write-1", "type": "function", "function": map[string]any{"name": "write_file", "arguments": string(args)}}}}, "finish_reason": "tool_calls"}}}
		data, _ := json.Marshal(payload)
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
	}))
	defer server.Close()
	invocationFixture(t)
	for _, enabled := range []bool{true, false} {
		flag.CommandLine = flag.NewFlagSet("hand", flag.ContinueOnError)
		flag.CommandLine.SetOutput(io.Discard)
		os.Args = []string{"hand", "--model=local/fixture", "--base-url=" + server.URL + "/v1", "-p", "write the file"}
		if enabled {
			os.Args = append(os.Args, "--dangerously-skip-permissions")
		}
		if err := run(); err != nil {
			t.Fatal(err)
		}
		_, err := os.Stat("approved.txt")
		if enabled {
			if err != nil {
				t.Fatal("flag did not approve the file tool", err)
			}
			if err = os.Remove("approved.txt"); err != nil {
				t.Fatal(err)
			}
		} else if !os.IsNotExist(err) {
			t.Fatal("approval survived a new launch without the flag", err)
		}
	}
}
