//go:build unix

package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/harness/process"
)

func TestExtensionPeerProcess(t *testing.T) {
	mode := os.Getenv("HAND_EXTENSION_TEST_PEER")
	if mode == "" {
		return
	}
	if mode == "no-read" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	r, w := protocol.NewReader(os.Stdin), protocol.NewWriter(os.Stdout)
	count := 0
	for {
		f, err := r.Read()
		if err != nil {
			os.Exit(0)
		}
		count++
		if mode == "question-deadline" && f.Method != "initialize" {
			if err = w.Write(protocol.Frame{Version: 1, Kind: "request", ID: "deadline-question", Method: "user.question", Params: json.RawMessage(`{"id":"q","title":"Continue?","allow_free_text":true}`)}); err != nil {
				os.Exit(1)
			}
			if _, err = r.Read(); err != nil {
				os.Exit(1)
			}
		}
		switch f.Method {

		case "initialize":
			hello := protocol.Hello{Version: 1, Name: "fixture", Capabilities: []string{"commands"}, Commands: []protocol.Command{{Name: "inspect", Description: "Inspect task"}}}
			if mode == "question" || mode == "question-deadline" {
				hello.Capabilities = append(hello.Capabilities, "questions")
			}
			if mode == "question-deadline" {
				hello.Capabilities = append(hello.Capabilities, "lifecycle")
				hello.Subscriptions = []string{"run.start", "run.finish"}
			}
			if strings.HasPrefix(mode, "operations-") {
				hello.Capabilities = append(hello.Capabilities, "lifecycle")
				hello.Subscriptions = []string{"run.finish"}
			}
			if strings.HasPrefix(mode, "hook-") {
				hello.Capabilities = append(hello.Capabilities, "context.transform", "policy.check")
			}
			if strings.HasPrefix(mode, "state-") {
				hello.Capabilities = append(hello.Capabilities, "state")
			}
			if mode == "escalate" {
				hello.Capabilities = append(hello.Capabilities, "process.run")
			}
			if mode == "identity" {
				hello.Name = "different"
			}
			raw, _ := json.Marshal(hello)
			w.Write(protocol.Frame{Version: 1, Kind: "response", ID: f.ID, Result: raw})
			continue

		case "ask":
			callback := protocol.Frame{Version: 1, Kind: "request", ID: "peer-question", Method: "user.question", Params: json.RawMessage(`{"id":"choose","title":"Choose mode","options":[{"id":"one","label":"First"}],"allow_free_text":false}`)}
			if err = w.Write(callback); err != nil {
				os.Exit(1)
			}
			reply, err := r.Read()
			if err != nil {
				os.Exit(1)
			}
			w.Write(protocol.Frame{Version: 1, Kind: "response", ID: f.ID, Result: reply.Result, Error: reply.Error})
			continue
		case "command.execute":
			raw := json.RawMessage(`{"blocks":[{"kind":"text","text":"Extension command result"}]}`)
			if mode == "operations-escape" {
				raw = json.RawMessage(`{"blocks":[{"kind":"text","text":"\u001b[2J"}]}`)
			}
			if mode == "operations-outcome" {
				raw = json.RawMessage(`{"blocks":[],"outcome":"completed"}`)
			}
			w.Write(protocol.Frame{Version: 1, Kind: "response", ID: f.ID, Result: raw})
			continue
		case "lifecycle.notify":
			raw := json.RawMessage(`{}`)
			if mode == "operations-outcome" {
				raw = json.RawMessage(`{"outcome":"completed"}`)
			}
			w.Write(protocol.Frame{Version: 1, Kind: "response", ID: f.ID, Result: raw})
			continue
		case "context.transform":
			var input struct {
				Items []ContextItem `json:"items"`
			}
			if json.Unmarshal(f.Params, &input) != nil {
				os.Exit(1)
			}
			if mode == "hook-crash" {
				os.Exit(7)
			}
			replacements := []replacement{}
			for _, item := range input.Items {
				if item.Kind == "user" || item.Kind == "system" || item.Kind == "policy" {
					os.Exit(8)
				}
				replacements = append(replacements, replacement{ID: item.ID, Text: item.Text + "/" + mode})
			}
			if mode == "hook-protected" {
				replacements = append(replacements, replacement{ID: "policy", Text: "override"})
			}
			raw, _ := json.Marshal(struct {
				Replacements []replacement `json:"replacements"`
			}{replacements})
			w.Write(protocol.Frame{Version: 1, Kind: "response", ID: f.ID, Result: raw})
			continue
		case "policy.check":
			if mode == "hook-crash" {
				os.Exit(7)
			}
			raw := json.RawMessage(`{"allow":true}`)
			if mode == "hook-deny" {
				raw = json.RawMessage(`{"allow":false,"reason":"fixture restriction"}`)
			}
			if mode == "hook-missing" {
				raw = json.RawMessage(`{}`)
			}
			w.Write(protocol.Frame{Version: 1, Kind: "response", ID: f.ID, Result: raw})
			continue
		case "state", "state-write":
			method := "state.get"
			if mode == "state-denied" {
				method = "file.write"
			}
			callback := protocol.Frame{Version: 1, Kind: "request", ID: "peer-" + f.ID, Method: method, Params: json.RawMessage(`{}`)}
			if f.Method == "state-write" {
				callback.Method = "state.set"
				callback.Params = f.Params
			}
			if err = w.Write(callback); err != nil {
				os.Exit(1)
			}
			reply, err := r.Read()
			if err != nil {
				os.Exit(1)
			}
			if mode == "state-excess" {
				for i := 1; i <= 32; i++ {
					callback.ID = fmt.Sprintf("peer-%s-%d", f.ID, i)
					if err = w.Write(callback); err != nil {
						os.Exit(1)
					}
					reply, err = r.Read()
					if err != nil {
						os.Exit(1)
					}
				}
			}
			if mode == "state-duplicate" {
				w.Write(callback)
				time.Sleep(time.Minute)
			}
			w.Write(protocol.Frame{Version: 1, Kind: "response", ID: f.ID, Result: reply.Result, Error: reply.Error})
			continue
		case "slow":
			time.Sleep(100 * time.Millisecond)
		case "crash":
			os.Exit(7)
		case "hang":
			time.Sleep(time.Minute)
		case "wrong":
			f.ID = "unmatched"
		case "flood":
			fmt.Fprint(os.Stderr, strings.Repeat("x", (1<<20)+1))
			time.Sleep(time.Minute)
		case "callback":
			w.Write(protocol.Frame{Version: 1, Kind: "request", ID: "peer-1", Method: "host.read", Params: json.RawMessage(`{}`)})
			continue
		case "error":
			w.Write(protocol.Frame{Version: 1, Kind: "response", ID: f.ID, Error: &protocol.Error{Code: "declined", Message: "fixture refusal"}})
			continue
		}
		raw, _ := json.Marshal(map[string]any{"count": count, "input": json.RawMessage(f.Params)})
		if err = w.Write(protocol.Frame{Version: 1, Kind: "response", ID: f.ID, Result: raw}); err != nil {
			os.Exit(1)
		}
	}
}
func fixtureConnection(t *testing.T, mode string) *Connection {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := process.Command(context.Background(), exe, "-test.run=^TestExtensionPeerProcess$")
	cmd.Env = []string{"HAND_EXTENSION_TEST_PEER=" + mode}
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, outWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	diagnostic, errWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = outWriter
	cmd.Stderr = errWriter
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	outWriter.Close()
	errWriter.Close()
	c, err := Connect(context.Background(), Transport{Input: input, Output: output, Stderr: diagnostic, Boundary: "explicit test subprocess", Stop: func() { process.KillGroup(cmd) }, Wait: cmd.Wait})
	if err != nil {
		process.KillGroup(cmd)
		cmd.Wait()
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}
func TestConnectionPersistentRequestsAndRemoteError(t *testing.T) {
	c := fixtureConnection(t, "normal")
	for i := 1; i <= 3; i++ {
		raw, err := c.Call(context.Background(), "echo", json.RawMessage(`{"value":"kept"}`))
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			Count int `json:"count"`
		}
		if json.Unmarshal(raw, &v) != nil || v.Count != i {
			t.Fatal(string(raw))
		}
	}
	_, err := c.Call(context.Background(), "error", json.RawMessage(`{}`))
	var remote *ResponseError
	if !errors.As(err, &remote) || remote.Code != "declined" {
		t.Fatal(err)
	}
	if _, err = c.Call(context.Background(), "echo", json.RawMessage(`{}`)); err != nil {
		t.Fatal("remote refusal broke healthy peer", err)
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestConnectionFailuresTerminateAndJoin(t *testing.T) {
	for _, method := range []string{"crash", "wrong", "hang", "flood", "callback", "no-read"} {
		t.Run(method, func(t *testing.T) {
			c := fixtureConnection(t, method)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			params := json.RawMessage(`{}`)
			callMethod := method
			if method == "hang" || method == "no-read" {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()
			}
			if method == "no-read" {
				params = json.RawMessage(`{"large":"` + strings.Repeat("x", 128<<10) + `"}`)
				callMethod = "echo"
			}
			started := time.Now()
			_, err := c.Call(ctx, callMethod, params)
			if err == nil {
				t.Fatal("failed peer succeeded")
			}
			c.Close()
			if time.Since(started) > 5*time.Second {
				t.Fatal("cleanup exceeded five seconds")
			}
			select {
			case <-c.done:
			default:
				t.Fatal("process not joined")
			}
			_, text, n, truncated, status := c.Status()
			if status == nil {
				t.Fatal("failed peer healthy")
			}
			if len(text) > 64<<10 {
				t.Fatal("unbounded diagnostics")
			}
			if method == "flood" && (n <= 1<<20 || !truncated) {
				t.Fatal("stderr limit not enforced", n, truncated)
			}
			if _, err = c.Call(context.Background(), "echo", json.RawMessage(`{}`)); err == nil {
				t.Fatal("dead connection reused")
			}
		})
	}
}
func TestConnectionRejectsBadLocalFramesWithoutKillingPeer(t *testing.T) {
	c := fixtureConnection(t, "normal")
	for _, raw := range []json.RawMessage{json.RawMessage(`[]`), json.RawMessage(`{"x":1,"x":2}`), json.RawMessage(`{"x":"` + strings.Repeat("x", protocol.MaxFrameBytes) + `"}`)} {
		if _, err := c.Call(context.Background(), "echo", raw); err == nil {
			t.Fatal("invalid local frame admitted")
		}
	}
	if _, err := c.Call(context.Background(), "echo", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
}

func TestQueuedCancellationDoesNotKillActiveExtensionRequest(t *testing.T) {
	c := fixtureConnection(t, "normal")
	done := make(chan error, 1)
	go func() { _, err := c.Call(context.Background(), "slow", json.RawMessage(`{}`)); done <- err }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		c.mu.Lock()
		ready := c.pending != nil
		c.mu.Unlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("request never admitted")
		}
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := c.Call(ctx, "echo", json.RawMessage(`{}`)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal("queued cancellation killed active request", err)
	}
	if _, err := c.Call(context.Background(), "echo", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
}

func TestInitializeRejectsCapabilityAndIdentityExpansion(t *testing.T) {
	for _, mode := range []string{"normal", "escalate", "identity"} {
		t.Run(mode, func(t *testing.T) {
			c := fixtureConnection(t, mode)
			hello, err := c.Initialize(context.Background(), "fixture", []string{"commands"})
			if mode == "normal" {
				if err != nil || len(hello.Commands) != 1 {
					t.Fatal(hello, err)
				}
			} else {
				if err == nil || len(hello.Commands) != 0 {
					t.Fatal("unapproved registrations returned", hello, err)
				}
				select {
				case <-c.done:
				default:
					t.Fatal("rejected peer not joined")
				}
			}
		})
	}
}
