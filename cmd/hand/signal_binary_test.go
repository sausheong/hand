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
	"syscall"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/protocol"
)

func TestBinarySignalsCancelStreamingRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "hand")
	if out, err := exec.CommandContext(ctx, "go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			ready, disconnected := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(disconnected)
				if _, err := io.Copy(io.Discard, r.Body); err != nil {
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"started\"},\"finish_reason\":null}]}\n\n")
				w.(http.Flusher).Flush()
				close(ready)
				<-r.Context().Done()
			}))
			defer server.Close()
			defer server.CloseClientConnections()
			cmd := exec.CommandContext(ctx, binary, "--model=local/fixture", "--base-url="+server.URL+"/v1", "--jsonl", "-p", "wait for cancellation")
			cmd.Dir = t.TempDir()
			cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer cmd.Process.Kill()
			waited := make(chan error, 1)
			go func() { waited <- cmd.Wait() }()
			select {
			case <-ready:
			case err := <-waited:
				t.Fatalf("exited before provider: %v %s", err, stderr.String())
			case <-ctx.Done():
				t.Fatal("provider not reached")
			}
			start := time.Now()
			if err := cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-waited:
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 130 {
					t.Fatalf("exit %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
				}
			case <-time.After(5 * time.Second):
				t.Fatal("signal shutdown exceeded five seconds")
			}
			select {
			case <-disconnected:
			case <-time.After(time.Second):
				t.Fatal("provider connection remains open after process exit")
			}
			reader := protocol.NewReader(&stdout)
			terminal := 0
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
				if terminal > 0 || event.Sequence <= sequence {
					t.Fatalf("unordered or post-terminal event: %+v", event)
				}
				sequence = event.Sequence
				if event.Kind == "terminal" {
					terminal++
					var outcome struct {
						Status   string `json:"status"`
						Verified bool   `json:"verified"`
					}
					if err := json.Unmarshal(event.Payload, &outcome); err != nil {
						t.Fatal(err)
					}
					if outcome.Status != string(agentio.Cancelled) || outcome.Verified {
						t.Fatalf("wrong terminal: %s", event.Payload)
					}
				}
			}
			if terminal != 1 {
				t.Fatalf("terminal count %d", terminal)
			}
			t.Logf("signal=%v shutdown_ms=%.3f provider_disconnected=true exit=130 terminal=1", sig, float64(time.Since(start).Microseconds())/1000)
		})
	}
}
