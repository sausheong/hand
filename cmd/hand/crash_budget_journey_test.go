//go:build darwin || linux

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
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/hand/sdk"
)

func TestBinaryCrashResumeRetainsBudgetReservation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "hand")
	if out, err := exec.CommandContext(ctx, "go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	ready, disconnected := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"budget journey answer\"},\"finish_reason\":null}]}\n\n")
		w.(http.Flusher).Flush()
		if n == 1 {
			close(ready)
			<-r.Context().Done()
			close(disconnected)
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	defer server.CloseClientConnections()
	home, workspace := t.TempDir(), t.TempDir()
	pidPath := filepath.Join(home, "hand.pid")
	start := func(sessionID string) *sdk.Client {
		t.Helper()
		flags := []string{"--rpc", "--model=local/fixture", "--base-url=" + server.URL + "/v1"}
		if sessionID != "" {
			flags = append(flags, "--session="+sessionID)
		}
		args := append([]string{"-c", `echo $$ > "$1"; shift; exec "$@"`, "fixture", pidPath, binary}, flags...)
		client, err := sdk.StartProcess(ctx, sdk.ProcessOptions{Binary: "/bin/sh", Arguments: args, Directory: workspace, Environment: append(os.Environ(), "HOME="+home)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { client.Close() })
		if _, err = client.Hello(ctx, "hello"); err != nil {
			t.Fatal(err)
		}
		return client
	}
	first := start("")
	if _, err := first.Call(ctx, "initial-budget", "budget.tokens.decide", map[string]any{"limit": 100000, "confirmed": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Prompt(ctx, "interrupted", "hold this request until interrupted"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		t.Fatalf("PID: %s %v", data, err)
	}
	if err = syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	if err = first.Close(); err == nil {
		t.Fatal("killed child reported clean shutdown")
	}
	select {
	case <-disconnected:
	case <-ctx.Done():
		t.Fatal("crash left provider connected")
	}
	catalogue, err := sessionio.OpenCatalogue(filepath.Join(home, ".hand", "sessions"), workspace)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := catalogue.Snapshot()
	if err != nil || saved.LastActiveID == "" {
		t.Fatalf("session missing: %+v %v", saved, err)
	}
	second := start(saved.LastActiveID)
	readBudget := func(client *sdk.Client) (int64, int, int) {
		t.Helper()
		raw, err := client.Call(ctx, "budget-view", "budget.tokens", nil)
		if err != nil {
			t.Fatal(err)
		}
		var view struct {
			Limit     int64 `json:"limit"`
			Committed int64 `json:"committed"`
			Attempts  int   `json:"attempts"`
			Uncertain int   `json:"uncertain_attempts"`
		}
		if err = json.Unmarshal(raw, &view); err != nil {
			t.Fatal(err)
		}
		return view.Committed, view.Attempts, view.Uncertain
	}
	committed, attempts, uncertain := readBudget(second)
	if committed <= 0 || attempts != 1 || uncertain != 1 {
		t.Fatalf("crash lost reservation: %d %d %d", committed, attempts, uncertain)
	}
	raw, err := second.Call(ctx, "old-request", "request.get", map[string]string{"id": "interrupted"})
	if err != nil || !strings.Contains(string(raw), `"state":"uncertain"`) {
		t.Fatalf("crash was not uncertain: %s %v", raw, err)
	}
	if _, err = second.Call(ctx, "ceiling", "budget.tokens.decide", map[string]any{"limit": committed + 1, "confirmed": true}); err != nil {
		t.Fatal(err)
	}
	if err = second.Close(); err != nil {
		t.Fatal(err)
	}
	third := start(saved.LastActiveID)
	c, a, u := readBudget(third)
	if c != committed || a != attempts || u != uncertain {
		t.Fatalf("restart changed charges: %d %d %d", c, a, u)
	}
	wait := func(id, want string) {
		t.Helper()
		for {
			record, err := third.Lookup(ctx, "lookup", id)
			if err != nil {
				t.Fatal(err)
			}
			if terminal, done := record.Terminal(); done {
				if terminal.Status() != want {
					t.Fatalf("status: %s want %s", terminal.Status(), want)
				}
				return
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(time.Millisecond):
			}
		}
	}
	if _, err = third.Prompt(ctx, "blocked", "continue without increasing budget"); err != nil {
		t.Fatal(err)
	}
	wait("blocked", "budget_exhausted")
	if calls.Load() != 1 {
		t.Fatalf("exhausted budget dispatched: %d", calls.Load())
	}
	if _, err = third.Call(ctx, "increase", "budget.tokens.decide", map[string]any{"limit": committed + 100000, "confirmed": true}); err != nil {
		t.Fatal(err)
	}
	if _, err = third.Prompt(ctx, "resumed", "continue with explicitly increased budget"); err != nil {
		t.Fatal(err)
	}
	wait("resumed", "completed")
	if calls.Load() != 2 {
		t.Fatalf("unexpected requests: %d", calls.Load())
	}
	c, a, u = readBudget(third)
	if c < committed || a < 2 || u < 1 {
		t.Fatalf("resumption erased uncertain charge: %d %d %d", c, a, u)
	}
	if err = third.Close(); err != nil {
		t.Fatal(err)
	}
}
