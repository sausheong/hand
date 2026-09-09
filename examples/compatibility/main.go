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
	var requests, cancelled atomic.Int32
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
		cancellation := false
		for _, m := range body.Messages {
			if m.Role == "user" && strings.Contains(string(m.Content), prompt) {
				found = true
				cancellation = strings.Contains(string(m.Content), "cancellation")
			}
		}
		if body.Model != "fixture" || !found {
			failures <- errors.New("model or prompt mismatch")
			http.Error(w, "mismatch", 400)
			return
		}
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		if cancellation {
			fmt.Fprint(w, `data: {"choices":[{"index":0,"delta":{"content":"cancel started"},"finish_reason":null}]}

`)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			cancelled.Add(1)
			return
		}

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
	cmd.Env = []string{"HOME=" + cliHome + "-home", "PATH=/usr/bin:/bin"}
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
		if _, err = observeCLI(&cli, scanner.Bytes()); err != nil {
			return err
		}
	}
	if err = scanner.Err(); err != nil {
		return err
	}
	results["cli_jsonl"] = cli
	rpcHome, err := directory("rpc")
	if err != nil {
		return err
	}
	client, err := sdk.StartProcess(ctx, sdk.ProcessOptions{Binary: binary, Arguments: []string{"--rpc", "--model", "local/fixture", "--base-url", server.URL + "/v1"}, Directory: rpcHome, Environment: []string{"HOME=" + rpcHome + "-home", "PATH=/usr/bin:/bin"}})
	if err != nil {
		return err
	}
	result, err := exercise(ctx, client, false)
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
	result, err = exercise(ctx, client, false)
	closeErr = client.Close()
	if err = errors.Join(err, closeErr); err != nil {
		return fmt.Errorf("embedded SDK: %w", err)
	}
	results["embedded_sdk"] = result
	for _, kind := range []string{"cli_jsonl", "subprocess_sdk", "embedded_sdk"} {
		home, e := directory(kind + "-cancel")
		if e != nil {
			return e
		}
		var observed observation
		if kind == "cli_jsonl" {
			observed, e = cancelCLI(ctx, binary, home, server.URL+"/v1")
		} else {
			var c *sdk.Client
			if kind == "subprocess_sdk" {
				c, e = sdk.StartProcess(ctx, sdk.ProcessOptions{Binary: binary, Arguments: []string{"--rpc", "--model", "local/fixture", "--base-url", server.URL + "/v1"}, Directory: home, Environment: []string{"HOME=" + home + "-home", "PATH=/usr/bin:/bin"}})
			} else {
				c, e = sdk.Open(ctx, sdk.EmbeddedOptions{Workspace: home, StoreDirectory: filepath.Join(home, "store"), Model: "local/fixture", Endpoint: server.URL + "/v1"})
			}
			if e == nil {
				observed, e = exercise(ctx, c, true)
				e = errors.Join(e, c.Close())
			}
		}
		if e != nil {
			return fmt.Errorf("%s cancellation: %w", kind, e)
		}
		if observed.Text != "cancel started" || observed.Status != "cancelled" || observed.Terminals != 1 {
			return fmt.Errorf("bad cancellation %+v", observed)
		}
		results[kind+"_cancel"] = observed
	}
	deadline := time.Now().Add(2 * time.Second)
	for cancelled.Load() != 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if cancelled.Load() != 3 {
		return fmt.Errorf("provider cancellation count %d", cancelled.Load())
	}

	select {
	case err = <-failures:
		return err
	default:
	}
	if requests.Load() != 6 {
		return fmt.Errorf("expected six provider calls, got %d", requests.Load())
	}
	for name, r := range results {
		if strings.HasSuffix(name, "_cancel") {
			continue
		}
		if r.Text != "compatible answer" || r.Status != "completed" || r.Terminals != 1 {
			return fmt.Errorf("unexpected %s observation: %+v", name, r)
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "passed", "provider_calls": requests.Load(), "observations": results, "cancelled_provider_requests": cancelled.Load(), "scope": "local fixture completion and cancellation journeys; excludes live evaluation"})
}

// Completion and cancellation must validate the same event envelope before
// counting any output toward the compatibility result.
func observeCLI(observed *observation, line []byte) (string, error) {
	var event protocol.Event
	if err := json.Unmarshal(line, &event); err != nil {
		return "", err
	}
	if event.Version != protocol.Version || event.RequestID == "" || event.RunID == "" || event.Kind == "" {
		return "", errors.New("CLI event lacks version, identity or kind")
	}
	var payload struct{ Text, Status string }
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return "", err
	}
	observe(observed, event.Kind, payload.Text, payload.Status)
	return event.Kind, nil
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
func exercise(ctx context.Context, client *sdk.Client, cancellation bool) (observation, error) {
	var result observation
	if _, err := client.Hello(ctx, "hello"); err != nil {
		return result, err
	}
	text := prompt
	if cancellation {
		text += " cancellation"
	}
	request, err := sdk.NewPromptRequest("run", text)
	if err != nil {
		return result, err
	}
	if _, err = client.Submit(ctx, request); err != nil {
		return result, err
	}
	var cursor uint64
	sentCancel := false
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
		if cancellation && !sentCancel && result.Text != "" {
			if _, err = client.Cancel(ctx, "cancel"); err != nil {
				return result, err
			}
			sentCancel = true
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

func cancelCLI(ctx context.Context, binary, home, endpoint string) (observation, error) {
	var observed observation
	cmd := exec.CommandContext(ctx, binary, "--model", "local/fixture", "--base-url", endpoint, "--jsonl", "-p", prompt+" cancellation")
	cmd.Dir = home
	cmd.Env = []string{"HOME=" + home + "-home", "PATH=/usr/bin:/bin"}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.StdoutPipe()
	if err != nil {
		return observed, err
	}
	if err = cmd.Start(); err != nil {
		return observed, err
	}
	defer func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), protocol.MaxFrameBytes)
	signalled := false
	for scanner.Scan() {
		kind, err := observeCLI(&observed, scanner.Bytes())
		if err != nil {
			return observed, err
		}
		if kind == "text" && !signalled {
			if err = cmd.Process.Signal(os.Interrupt); err != nil {
				return observed, err
			}
			signalled = true
		}
	}
	if err = scanner.Err(); err != nil {
		return observed, err
	}
	err = cmd.Wait()
	var exited *exec.ExitError
	if !signalled || !errors.As(err, &exited) || exited.ExitCode() != 130 {
		return observed, fmt.Errorf("expected interrupted CLI exit 130, got %v: %s", err, stderr.String())
	}
	return observed, nil
}
