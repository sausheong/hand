package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/config"
)

func TestBinaryMandatoryToolTimeoutPreventsMutation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "hand")
	if out, err := exec.CommandContext(ctx, "go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	home, workspace := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".hand"), 0700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(home, ".hand", "config.json")
	marker := filepath.Join(workspace, "validator-entered")
	target := filepath.Join(workspace, ".hand", "skills", "timeout-skill", "SKILL.md")
	var calls atomic.Int32
	requests := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		requests <- string(body)
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1)%2 == 1 {
			args, _ := json.Marshal(map[string]string{"action": "create", "name": "timeout-skill", "body": "approved content"})
			event := map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "create", "type": "function", "function": map[string]any{"name": "skill_manage", "arguments": string(args)}}}}, "finish_reason": nil}}}
			raw, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", raw)
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
		} else {
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"finished\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}
	}))
	defer server.Close()
	for _, timedOut := range []bool{true, false} {
		script := `printf entered > "$1"`
		if timedOut {
			script += `; exec sleep 10`
		}
		cfg := config.Config{Hooks: []config.HookConfig{{Event: "PreToolUse", Matcher: "skill_manage", Command: "sh", Args: []string{"-c", script, "validator", marker}, Timeout: 1}}}
		raw, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(configPath, raw, 0600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, binary, "--model=local/fixture", "--base-url="+server.URL+"/v1", "--new-session", "--yes", "-p", "create the skill")
		cmd.Dir = workspace
		cmd.Env = append(os.Environ(), "HOME="+home)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err, string(out))
		}
		wantCalls := int32(2)
		if !timedOut {
			wantCalls = 4
		}
		if calls.Load() != wantCalls {
			t.Fatalf("provider calls %d, want %d", calls.Load(), wantCalls)
		}
		if data, err := os.ReadFile(marker); err != nil || string(data) != "entered" {
			t.Fatal("validator did not run", err)
		}
		<-requests
		followup := <-requests
		data, err := os.ReadFile(target)
		if timedOut {
			if !os.IsNotExist(err) {
				t.Fatal("mandatory timeout allowed mutation", err, string(data))
			}
			if !strings.Contains(followup, "timed out") {
				t.Fatal("provider did not receive timeout denial", followup)
			}
			if err := os.Remove(marker); err != nil {
				t.Fatal(err)
			}
		} else if err != nil || !strings.Contains(string(data), "approved content") {
			t.Fatal("successful validator did not permit valid mutation", err, string(data))
		}
	}
	if calls.Load() != 4 {
		t.Fatal("unexpected provider calls", calls.Load())
	}
}
