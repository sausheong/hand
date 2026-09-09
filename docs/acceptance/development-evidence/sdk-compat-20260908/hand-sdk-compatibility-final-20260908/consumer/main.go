// Command compatibility compares Hand interfaces using a local HTTP provider.
// It is copied into an external module by scripts/check_sdk_compatibility.py.
package main

import (
	"bufio"
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
	"strings"
	"sync/atomic"
	"time"

	"github.com/sausheong/hand/protocol"
	"github.com/sausheong/hand/sdk"
)

const prompt = "verify interface compatibility"

type observation struct {
	Text      string `json:"text"`
	Status    string `json:"status"`
	Terminals int    `json:"terminals"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: compatibility HAND_BINARY")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(binary string) error {
	root, err := os.MkdirTemp("", "hand-interface-compatibility-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	var requests atomic.Int32
	failures := make(chan error, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "unexpected endpoint", 400)
			return
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&body); err != nil {
			failures <- err
			http.Error(w, "bad request", 400)
			return
		}
		found := false
		for _, m := range body.Messages {
			if m.Role == "user" && strings.Contains(string(m.Content), prompt) {
				found = true
			}
		}
		if body.Model != "fixture" || !found {
			failures <- errors.New("model or prompt mismatch")
			http.Error(w, "mismatch", 400)
			return
		}
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"index":0,"delta":{"content":"compatible answer"},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	results := make(map[string]observation)
	directory := func(name string) (string, error) { p := filepath.Join(root, name); return p, os.Mkdir(p, 0700) }
	cliHome, err := directory("cli")
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, binary, "--model", "local/fixture", "--base-url", server.URL+"/v1", "--jsonl", "-p", prompt)
	cmd.Dir = cliHome
	cmd.Env = []string{"HOME=" + cliHome, "PATH=/usr/bin:/bin"}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("CLI failed: %w: %s", err, stderr.String())
	}
	var cli observation
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 4096), protocol.MaxFrameBytes)
	for scanner.Scan() {
		var event protocol.Event
		if err = json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return err
		}
		if event.Version != 1 || event.RequestID == "" || event.RunID == "" {
			return errors.New("CLI event lacks identity")
		}
		var payload struct{ Text, Status string }
		if err = json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		observe(&cli, event.Kind, payload.Text, payload.Status)
	}
	if err = scanner.Err(); err != nil {
		return err
	}
	results["cli_jsonl"] = cli
	rpcHome, err := directory("rpc")
	if err != nil {
		return err
	}
	client, err := sdk.StartProcess(ctx, sdk.ProcessOptions{Binary: binary, Arguments: []string{"--rpc", "--model", "local/fixture", "--base-url", server.URL + "/v1"}, Directory: rpcHome, Environment: []string{"HOME=" + rpcHome, "PATH=/usr/bin:/bin"}})
	if err != nil {
		return err
	}
	result, err := exercise(ctx, client)
	closeErr := client.Close()
	if err = errors.Join(err, closeErr); err != nil {
		return fmt.Errorf("subprocess SDK: %w", err)
	}
	results["subprocess_sdk"] = result
	embeddedHome, err := directory("embedded")
	if err != nil {
		return err
	}
	client, err = sdk.Open(ctx, sdk.EmbeddedOptions{Workspace: embeddedHome, StoreDirectory: filepath.Join(embeddedHome, "store"), Model: "local/fixture", Endpoint: server.URL + "/v1"})
	if err != nil {
		return err
	}
	result, err = exercise(ctx, client)
	closeErr = client.Close()
	if err = errors.Join(err, closeErr); err != nil {
		return fmt.Errorf("embedded SDK: %w", err)
	}
	results["embedded_sdk"] = result
	select {
	case err = <-failures:
		return err
	default:
	}
	if requests.Load() != 3 {
		return fmt.Errorf("expected three provider calls, got %d", requests.Load())
	}
	for name, r := range results {
		if r.Text != "compatible answer" || r.Status != "completed" || r.Terminals != 1 {
			return fmt.Errorf("unexpected %s observation: %+v", name, r)
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "passed", "provider_calls": requests.Load(), "observations": results, "scope": "local fixture completion journey; excludes live evaluation"})
}
func observe(o *observation, kind, text, status string) {
	if kind == "text" {
		o.Text += text
	}
	if kind == "terminal" {
		o.Terminals++
		o.Status = status
	}
}
func exercise(ctx context.Context, client *sdk.Client) (observation, error) {
	var result observation
	if _, err := client.Hello(ctx, "hello"); err != nil {
		return result, err
	}
	request, err := sdk.NewPromptRequest("run", prompt)
	if err != nil {
		return result, err
	}
	if _, err = client.Submit(ctx, request); err != nil {
		return result, err
	}
	var cursor uint64
	for {
		page, err := client.PollEvents(ctx, "poll", cursor)
		if err != nil {
			return result, err
		}
		if page.Gap() {
			return result, errors.New("unexpected progress gap")
		}
		for _, entry := range page.Events() {
			event := entry.Event()
			if event.RequestID() != "run" {
				return result, errors.New("request identity changed")
			}
			observe(&result, event.Kind(), event.Text(), event.Status())
		}
		cursor = page.Next()
		if result.Terminals > 0 {
			record, err := client.Lookup(ctx, "lookup", "run")
			if err != nil {
				return result, err
			}
			terminal, ok := record.Terminal()
			if !ok || terminal.Status() != result.Status {
				return result, errors.New("durable terminal differs from event stream")
			}
			return result, nil
		}
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}
