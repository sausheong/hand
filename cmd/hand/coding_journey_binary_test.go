package main

import (
	"bytes"
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
	"sync"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/hand/protocol"
	"github.com/sausheong/hand/sdk"
)

// A scripted provider controls decisions, while the compiled client performs
// actual writes, subprocess tests, durable session storage and restart.
func TestBinaryCodingEditTestResumeJourney(t *testing.T)    { runBinaryCodingJourney(t, false) }
func TestBinaryRPCCodingEditTestResumeJourney(t *testing.T) { runBinaryCodingJourney(t, true) }

func runBinaryCodingJourney(t *testing.T, rpcMode bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "hand")
	if out, err := exec.CommandContext(ctx, "go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	home, workspace := t.TempDir(), t.TempDir()
	checkpointDir := filepath.Join(t.TempDir(), "checkpoints")
	var firstRunID string
	original := "package sum\nfunc Add(a, b int) int { return a - b }\n"
	fixed := "package sum\nfunc Add(a, b int) int { return a + b }\n"
	for name, content := range map[string]string{
		"go.mod":         "module journey.example/sum\n\ngo 1.23\n",
		"sum.go":         original,
		"sum_test.go":    "package sum\nimport \"testing\"\nfunc TestAdd(t *testing.T) { if Add(2,3) != 5 { t.Fatal(\"addition broken\") } }\n",
		"user-notes.txt": "pre-existing user content\n",
	} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	baseline := exec.CommandContext(ctx, "go", "test", "./...")
	baseline.Dir = workspace
	if out, err := baseline.CombinedOutput(); err == nil || !strings.Contains(string(out), "addition broken") {
		t.Fatalf("oracle must reject original: %v %s", err, out)
	}
	var mu sync.Mutex
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		if err != nil {
			http.Error(w, "read", 400)
			return
		}
		mu.Lock()
		requests = append(requests, string(body))
		n := len(requests)
		mu.Unlock()
		delta := map[string]any{"content": "coding journey finished"}
		finish := "stop"
		name := ""
		args := map[string]string{}
		switch n {
		case 1:
			name = "write_file"
			args = map[string]string{"path": "sum.go", "content": fixed}
		case 2:
			name = "bash"
			args = map[string]string{"command": "go test ./... && printf 'tests passed\\n' > test-result.txt"}
		}
		if name != "" {
			encoded, _ := json.Marshal(args)
			delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("journey-%d", n), "type": "function", "function": map[string]any{"name": name, "arguments": string(encoded)}}}}
			finish = "tool_calls"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []any{map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}}, map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}}}} {
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	invoke := func(args ...string) {
		t.Helper()
		flags := append([]string{"--model=local/fixture", "--base-url=" + server.URL + "/v1", "--max-turns=6"}, args...)
		if rpcMode {
			var processArgs []string
			var prompt string
			for i := 0; i < len(flags); i++ {
				if flags[i] == "--yes" {
					continue
				}
				if flags[i] == "-p" {
					i++
					prompt = flags[i]
					continue
				}
				processArgs = append(processArgs, flags[i])
			}
			processArgs = append(processArgs, "--rpc", "--checkpoint-dir="+checkpointDir)
			var stderr bytes.Buffer
			client, err := sdk.StartProcess(ctx, sdk.ProcessOptions{Binary: binary, Arguments: processArgs, Directory: workspace, Environment: append(os.Environ(), "HOME="+home), Stderr: &stderr})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if _, err = client.Hello(ctx, "hello"); err != nil {
				t.Fatal(err)
			}
			id := "coding"
			if strings.Contains(prompt, "Summarise") {
				id = "resume"
			}
			if _, err = client.Prompt(ctx, id, prompt); err != nil {
				t.Fatal(err)
			}
			approvals := 0
			for {
				raw, err := client.Call(ctx, "pending", "approval.pending", nil)
				if err != nil {
					t.Fatal(err)
				}
				var pending struct{ Approvals []protocol.Event }
				if err = json.Unmarshal(raw, &pending); err != nil {
					t.Fatal(err)
				}
				for _, event := range pending.Approvals {
					if event.RequestID != id {
						t.Fatal("approval has wrong request identity")
					}
					var payload struct {
						ApprovalID string `json:"approval_id"`
					}
					if err = json.Unmarshal(event.Payload, &payload); err != nil {
						t.Fatal(err)
					}
					if _, err = client.Call(ctx, fmt.Sprintf("approve-%d", approvals), "approval.respond", map[string]string{"run_id": event.RunID, "approval_id": payload.ApprovalID, "decision": "once"}); err != nil {
						t.Fatal(err)
					}
					approvals++
				}
				record, err := client.Lookup(ctx, "lookup", id)
				if err != nil {
					t.Fatal(err)
				}
				if terminal, done := record.Terminal(); done {
					if id == "coding" {
						raw, e := client.Call(ctx, "coding-changes", "checkpoint.changes", nil)
						if e != nil {
							t.Fatal(e)
						}
						var changes struct {
							RunID string `json:"run_id"`
						}
						if e = json.Unmarshal(raw, &changes); e != nil || changes.RunID == "" {
							t.Fatalf("checkpoint identity missing: %s %v", raw, e)
						}
						firstRunID = changes.RunID
					}
					if terminal.Status() != "completed" {
						t.Fatalf("RPC terminal: %s %s", terminal.Status(), terminal.Reason())
					}
					want := 2
					if id == "resume" {
						want = 0
					}
					if approvals != want {
						t.Fatalf("approvals=%d want=%d", approvals, want)
					}
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(time.Millisecond):
				}
			}
			if err = client.Close(); err != nil {
				t.Fatalf("RPC shutdown: %v %s", err, stderr.String())
			}
			return
		}
		cmd := exec.CommandContext(ctx, binary, flags...)
		cmd.Dir = workspace
		cmd.Env = append(os.Environ(), "HOME="+home)
		if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), "coding journey finished") {
			t.Fatalf("invoke: %v %s", err, out)
		}
	}
	// --yes is explicit invocation approval; interactive approval has separate PTY coverage.
	invoke("--yes", "-p", "Repair Add and run the repository tests")
	if data, err := os.ReadFile(filepath.Join(workspace, "sum.go")); err != nil || string(data) != fixed {
		t.Fatalf("edit not applied: %q %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(workspace, "test-result.txt")); err != nil || string(data) != "tests passed\n" {
		t.Fatalf("real tests did not pass: %q %v", data, err)
	}
	catalogue, err := sessionio.OpenCatalogue(filepath.Join(home, ".hand", "sessions"), workspace)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := catalogue.Snapshot()
	if err != nil || len(saved.Sessions) != 1 || saved.LastActiveID == "" {
		t.Fatalf("session missing: %+v %v", saved, err)
	}
	// Preserve a user edit made between processes and prove it reaches no write tool.
	userContent := "user changed this after the coding run\n"
	if err := os.WriteFile(filepath.Join(workspace, "user-notes.txt"), []byte(userContent), 0600); err != nil {
		t.Fatal(err)
	}
	invoke("--session="+saved.LastActiveID, "-p", "Summarise the change and test result from the previous run")
	mu.Lock()
	captured := append([]string(nil), requests...)
	mu.Unlock()
	if len(captured) != 4 {
		t.Fatalf("expected write, test, answer, resume requests: %d", len(captured))
	}
	var resumed struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
	}
	if err := json.Unmarshal([]byte(captured[3]), &resumed); err != nil {
		t.Fatal(err)
	}
	tools := 0
	for _, m := range resumed.Messages {
		if m.Role == "tool" {
			tools++
		}
	}
	if tools != 2 || !strings.Contains(captured[3], "Repair Add") || !strings.Contains(captured[3], "coding journey finished") {
		t.Fatalf("resumed history lost task/tool results: %s", captured[3])
	}
	if data, err := os.ReadFile(filepath.Join(workspace, "user-notes.txt")); err != nil || string(data) != userContent {
		t.Fatalf("user edit changed: %q %v", data, err)
	}
	final, err := catalogue.Snapshot()
	if err != nil || len(final.Sessions) != 1 || final.LastActiveID != saved.LastActiveID {
		t.Fatalf("resume identity changed: %+v %v", final, err)
	}
	if rpcMode {
		client, err := sdk.StartProcess(ctx, sdk.ProcessOptions{Binary: binary, Arguments: []string{"--rpc", "--model=local/fixture", "--base-url=" + server.URL + "/v1", "--checkpoint-dir=" + checkpointDir, "--session=" + saved.LastActiveID}, Directory: workspace, Environment: append(os.Environ(), "HOME="+home)})
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		if _, err = client.Hello(ctx, "restore-hello"); err != nil {
			t.Fatal(err)
		}
		preview := func(id string) json.RawMessage {
			t.Helper()
			raw, err := client.Call(ctx, id, "checkpoint.restore_preview", map[string]any{"run_id": firstRunID, "paths": []string{"sum.go"}})
			if err != nil {
				t.Fatal(err)
			}
			return raw
		}
		clean := preview("clean-preview")
		var state struct {
			Conflicts bool `json:"conflicts"`
		}
		if err = json.Unmarshal(clean, &state); err != nil || state.Conflicts {
			t.Fatalf("clean preview: %s %v", clean, err)
		}
		userCode := fixed + "// subsequent user edit\n"
		if err = os.WriteFile(filepath.Join(workspace, "sum.go"), []byte(userCode), 0600); err != nil {
			t.Fatal(err)
		}
		// A valid preview becomes stale when the user edits the selected file.
		stale, err := client.Call(ctx, "stale-restore", "checkpoint.restore", map[string]any{"confirmed": true, "preview": clean})
		if err != nil {
			t.Fatal(err)
		}
		var refused struct {
			Completed bool   `json:"completed"`
			Error     string `json:"error"`
		}
		if err = json.Unmarshal(stale, &refused); err != nil || refused.Completed || refused.Error == "" {
			t.Fatalf("stale restore not refused: %s %v", stale, err)
		}
		conflict := preview("conflict-preview")
		if err = json.Unmarshal(conflict, &state); err != nil || !state.Conflicts {
			t.Fatalf("user edit not flagged: %s %v", conflict, err)
		}
		if data, err := os.ReadFile(filepath.Join(workspace, "sum.go")); err != nil || string(data) != userCode {
			t.Fatalf("user code overwritten: %q %v", data, err)
		}
		// The user explicitly returns the file to the recorded post-image.
		if err = os.WriteFile(filepath.Join(workspace, "sum.go"), []byte(fixed), 0600); err != nil {
			t.Fatal(err)
		}
		fresh := preview("fresh-preview")
		raw, err := client.Call(ctx, "confirmed-restore", "checkpoint.restore", map[string]any{"confirmed": true, "preview": fresh})
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Completed bool `json:"completed"`
		}
		if err = json.Unmarshal(raw, &result); err != nil || !result.Completed {
			t.Fatalf("restore incomplete: %s %v", raw, err)
		}
		if data, err := os.ReadFile(filepath.Join(workspace, "sum.go")); err != nil || string(data) != original {
			t.Fatalf("starting content lost: %q %v", data, err)
		}
		if data, err := os.ReadFile(filepath.Join(workspace, "user-notes.txt")); err != nil || string(data) != userContent {
			t.Fatalf("unselected user edit changed: %q %v", data, err)
		}
		if data, err := os.ReadFile(filepath.Join(workspace, "test-result.txt")); err != nil || string(data) != "tests passed\n" {
			t.Fatalf("unselected artifact changed: %q %v", data, err)
		}
		if err = client.Close(); err != nil {
			t.Fatal(err)
		}
	}

}
