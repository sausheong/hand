//go:build darwin || linux

package sdk_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sausheong/hand/protocol"
	"github.com/sausheong/hand/sdk"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestScopedSDKApprovalReuseRevokeAndDeny(t *testing.T) {
	runScopedSDKJourney(t, sdk.ExecutionOptions{})
}

func runScopedSDKJourney(t *testing.T, execution sdk.ExecutionOptions) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct {
			Messages []struct {
				Role    string
				Content json.RawMessage
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		toolResult := false
		text := ""
		for _, m := range body.Messages {
			if m.Role == "tool" {
				toolResult = true
			}
			if m.Role == "user" {
				json.Unmarshal(m.Content, &text)
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta any, reason any) {
			raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": reason}}})
			fmt.Fprintf(w, "data: %s\n\n", raw)
		}
		if toolResult {
			send(map[string]string{"content": "finished"}, nil)
			send(map[string]any{}, "stop")
		} else {
			args, _ := json.Marshal(map[string]string{"path": "scoped.txt", "content": text})
			send(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "write", "type": "function", "function": map[string]string{"name": "write_file", "arguments": string(args)}}}}, nil)
			send(map[string]any{}, "tool_calls")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	workspace := t.TempDir()
	client, err := sdk.Open(ctx, sdk.EmbeddedOptions{Execution: execution, Workspace: workspace, StoreDirectory: t.TempDir(), AuthorityDirectory: filepath.Join(t.TempDir(), "authority"), Model: "local/fixture", Endpoint: server.URL + "/v1"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	hello, err := client.Hello(ctx, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if execution.Backend == "container" && !strings.Contains(string(hello), "container:") {
		t.Fatal("SDK container boundary missing")
	}
	run := func(id, decision string, wantPrompts int) {
		t.Helper()
		if _, err = client.Prompt(ctx, id, id); err != nil {
			t.Fatal(err)
		}
		prompts := 0
		for {
			raw, e := client.Call(ctx, "pending", "approval.pending", nil)
			if e != nil {
				t.Fatal(e)
			}
			var pending struct{ Approvals []protocol.Event }
			if e = json.Unmarshal(raw, &pending); e != nil {
				t.Fatal(e)
			}
			for _, event := range pending.Approvals {
				prompts++
				if prompts > wantPrompts {
					t.Fatal("unexpected repeated approval")
				}
				if event.RequestID != id {
					t.Fatal("approval belongs to wrong run")
				}
				var payload struct {
					ApprovalID string `json:"approval_id"`
				}
				json.Unmarshal(event.Payload, &payload)
				if _, e = client.Call(ctx, id+"-decision", "approval.respond", map[string]string{"run_id": event.RunID, "approval_id": payload.ApprovalID, "decision": decision}); e != nil {
					t.Fatal(e)
				}
			}
			record, e := client.Lookup(ctx, "lookup", id)
			if e != nil {
				t.Fatal(e)
			}
			if _, done := record.Terminal(); done {
				if prompts != wantPrompts {
					t.Fatalf("got %d approvals, want %d", prompts, wantPrompts)
				}
				return
			}
			select {
			case <-ctx.Done():
				t.Fatal("run timed out")
			case <-time.After(time.Millisecond):
			}
		}
	}
	newSession := func(prefix string) {
		t.Helper()
		for attempt := 0; attempt < 100; attempt++ {
			_, e := client.Call(ctx, fmt.Sprintf("%s-%d", prefix, attempt), "session.new", nil)
			if e == nil {
				return
			}
			var detail *protocol.Error
			if !errors.As(e, &detail) || detail.Code != "operation_rejected" || detail.Message != "another application operation is active" {
				t.Fatal(e)
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("session remained busy")
	}
	run("first", "always", 1)
	data, err := os.ReadFile(filepath.Join(workspace, "scoped.txt"))
	if err != nil || string(data) != "first" {
		t.Fatalf("first approved write %q %v", data, err)
	}
	newSession("second-session")
	run("second", "deny", 0)
	data, err = os.ReadFile(filepath.Join(workspace, "scoped.txt"))
	if err != nil || string(data) != "second" {
		t.Fatalf("grant reuse write %q %v", data, err)
	}
	raw, err := client.Call(ctx, "permissions", "permission.list", nil)
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Grants []struct{ ID, Resource, Operation, Provenance string }
		Total  int
	}
	if err = json.Unmarshal(raw, &list); err != nil || list.Total != 1 {
		t.Fatalf("grant inspection %s %v", raw, err)
	}
	canonical, _ := filepath.EvalSymlinks(workspace)
	g := list.Grants[0]
	if g.Resource != filepath.Join(canonical, "scoped.txt") || g.Operation != "file.write" || g.Provenance != "user_decision" {
		t.Fatalf("wrong grant scope %+v", g)
	}
	if _, err = client.Call(ctx, "revoke", "permission.revoke", map[string]string{"id": g.ID}); err != nil {
		t.Fatal(err)
	}
	newSession("third-session")
	run("third", "deny", 1)
	data, err = os.ReadFile(filepath.Join(workspace, "scoped.txt"))
	if err != nil || string(data) != "second" {
		t.Fatalf("denied write changed file %q %v", data, err)
	}
	if calls.Load() != 6 {
		t.Fatalf("unexpected provider calls %d", calls.Load())
	}
	if err = client.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNativeScopedSDKContainerJourney(t *testing.T) {
	image, socket, worker := os.Getenv("HARNESS_TEST_BASH_IMAGE"), os.Getenv("HARNESS_TEST_CONTAINER_SOCKET"), os.Getenv("HAND_TEST_WORKER_BINARY")
	if image == "" || socket == "" || worker == "" {
		t.Skip("native SDK container fixture not configured")
	}
	raw, err := os.ReadFile(worker)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	runScopedSDKJourney(t, sdk.ExecutionOptions{Backend: "container", Docker: "/usr/local/bin/docker", Socket: socket, Image: image, Worker: worker, WorkerSHA256: hex.EncodeToString(hash[:]), Writable: true})
}

func TestSDKContainerRequiresAuthorityDirectory(t *testing.T) {
	client, err := sdk.Open(context.Background(), sdk.EmbeddedOptions{Workspace: t.TempDir(), StoreDirectory: t.TempDir(), Model: "local/fixture", Execution: sdk.ExecutionOptions{Backend: "container"}})
	if client != nil {
		client.Close()
		t.Fatal("client created without scoped authority")
	}
	if err == nil || !strings.Contains(err.Error(), "external authority directory") {
		t.Fatal(err)
	}
}
func TestNativeSDKRejectsChangedWorker(t *testing.T) {
	image, socket, worker := os.Getenv("HARNESS_TEST_BASH_IMAGE"), os.Getenv("HARNESS_TEST_CONTAINER_SOCKET"), os.Getenv("HAND_TEST_WORKER_BINARY")
	if image == "" || socket == "" || worker == "" {
		t.Skip("native SDK container fixture not configured")
	}
	client, err := sdk.Open(context.Background(), sdk.EmbeddedOptions{Workspace: t.TempDir(), StoreDirectory: t.TempDir(), AuthorityDirectory: filepath.Join(t.TempDir(), "authority"), Model: "local/fixture", Execution: sdk.ExecutionOptions{Backend: "container", Docker: "/usr/local/bin/docker", Socket: socket, Image: image, Worker: worker, WorkerSHA256: strings.Repeat("0", 64)}})
	if client != nil {
		client.Close()
		t.Fatal("client created with incorrect worker digest")
	}
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatal(err)
	}
}
