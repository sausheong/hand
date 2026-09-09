//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/sausheong/hand/protocol"
	"github.com/sausheong/hand/sdk"
)

func TestBinarySDKSteerDuringToolThenCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "hand")
	if out, err := exec.CommandContext(ctx, "go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		args, _ := json.Marshal(map[string]string{"command": "echo $$ > running.pid; exec sleep 30"})
		delta := map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "held-tool", "type": "function", "function": map[string]any{"name": "bash", "arguments": string(args)}}}}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []any{map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}}, map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls"}}}} {
			raw, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", raw)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	home, workspace := t.TempDir(), t.TempDir()
	client, err := sdk.StartProcess(ctx, sdk.ProcessOptions{Binary: binary, Arguments: []string{"--rpc", "--model=local/fixture", "--base-url=" + server.URL + "/v1"}, Directory: workspace, Environment: append(os.Environ(), "HOME="+home)})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err = client.Hello(ctx, "hello"); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Prompt(ctx, "run", "start the long tool"); err != nil {
		t.Fatal(err)
	}
	for {
		raw, err := client.Call(ctx, "pending", "approval.pending", nil)
		if err != nil {
			t.Fatal(err)
		}
		var pending struct{ Approvals []protocol.Event }
		if err = json.Unmarshal(raw, &pending); err != nil {
			t.Fatal(err)
		}
		if len(pending.Approvals) > 0 {
			if len(pending.Approvals) != 1 || pending.Approvals[0].RequestID != "run" {
				t.Fatal("wrong approval identity")
			}
			event := pending.Approvals[0]
			var payload struct {
				ApprovalID string `json:"approval_id"`
			}
			if err = json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if _, err = client.Call(ctx, "approve", "approval.respond", map[string]string{"run_id": event.RunID, "approval_id": payload.ApprovalID, "decision": "once"}); err != nil {
				t.Fatal(err)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	var pid int
	for pid == 0 {
		data, _ := os.ReadFile(filepath.Join(workspace, "running.pid"))
		pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	if err = syscall.Kill(pid, 0); err != nil {
		t.Fatalf("tool not running: %v", err)
	}
	if _, err = client.Call(ctx, "steering", "steer", map[string]string{"text": "use the revised direction after the tool"}); err != nil {
		t.Fatal(err)
	}
	queued, err := client.Call(ctx, "queue-before", "queue.list", nil)
	if err != nil || !strings.Contains(string(queued), "use the revised direction") {
		t.Fatalf("steering not retained during tool: %s %v", queued, err)
	}
	started := time.Now()
	if _, err = client.Cancel(ctx, "cancel"); err != nil {
		t.Fatal(err)
	}
	for {
		record, err := client.Lookup(ctx, "lookup", "run")
		if err != nil {
			t.Fatal(err)
		}
		if terminal, done := record.Terminal(); done {
			if terminal.Status() != "cancelled" {
				t.Fatalf("terminal: %s", terminal.Status())
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("cancel cleanup took %s", elapsed)
	}
	if err = syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("terminal preceded child cleanup: %v", err)
	}
	if err = client.Close(); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("cancel allowed another provider dispatch: %d", calls.Load())
	}
}
