package app

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/llm/llmtest"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
)

type extensionContextProvider struct {
	llmtest.Base
	requests []llm.ChatRequest
}

func (p *extensionContextProvider) ChatStream(_ context.Context, req llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	p.requests = append(p.requests, req)
	out := make(chan llm.ChatEvent, 1)
	out <- llm.ChatEvent{Type: llm.EventDone}
	close(out)
	return out, nil
}
func TestExtensionTransformsProviderContextPreservingJournal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "note")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./examples/extensions/go-task-note")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build %v %s", err, out)
	}
	for _, mode := range []string{"run", "runturn"} {
		t.Run(mode, func(t *testing.T) {
			sess := session.NewSession("hand", "context")
			defer sess.Close()
			sess.Append(session.UserMessageEntry("  protected user text  "))
			sess.Append(session.ToolCallEntry("read-1", "read_file", json.RawMessage(`{}`)))
			sess.Append(session.ToolResultEntry("read-1", "  original result  ", "", nil))
			before := sess.View()
			rawBefore, _ := json.Marshal(before)
			provider := &extensionContextProvider{}
			rt := &runtime.Runtime{Session: sess, LLM: provider, Tools: tool.NewRegistry(), StaticSystemPrompt: "protected system"}
			defer rt.Close()
			c := &Controller{Rt: rt}
			workspace := t.TempDir()
			review, err := extensions.ReviewHostLaunch(ctx, extensions.LaunchConfig{Name: "task-note", Executable: binary, Workspace: workspace, Capabilities: []string{"commands", "questions", "state", "context.transform"}})
			if err != nil {
				t.Fatal(err)
			}
			host, err := c.ActivateExtensions(ctx, ExtensionStartup{Version: 1, SnapshotRoot: filepath.Join(t.TempDir(), "private"), Identities: map[string]string{"task-note": "examples/note"}, Reviews: []extensions.LaunchReview{review}}, workspace, true)
			if err != nil {
				t.Fatal(err)
			}
			defer host.Close()
			if mode == "run" {
				events, err := rt.Run(ctx, "finish", nil)
				if err != nil {
					t.Fatal(err)
				}
				for event := range events {
					if event.Error != nil {
						t.Fatal(event.Error)
					}
				}
			} else {
				result, err := rt.RunTurn(ctx, "finish", nil, nil)
				if err != nil || result.Err != nil {
					t.Fatal(result, err)
				}
			}
			if len(provider.requests) != 1 {
				t.Fatalf("provider calls %d", len(provider.requests))
			}
			found := false
			userFound := false
			for _, message := range provider.requests[0].Messages {
				if message.ToolCallID == "read-1" {
					found = true
					if message.Content != "original result" {
						t.Fatalf("transform not applied: %q", message.Content)
					}
				}
				if message.Content == "  protected user text  " {
					userFound = true
				}
			}
			if !found || !userFound {
				t.Fatal("tool or protected user context missing")
			}
			many := make([]runtime.ToolContextText, 130)
			for i := range many {
				many[i] = runtime.ToolContextText{Index: i, Text: " trim "}
			}
			transformed, err := host.transformToolContext(ctx, many)
			if err != nil || len(transformed) != len(many) {
				t.Fatal("batched transform", err)
			}
			for i, item := range transformed {
				if item.Index != i || item.Text != "trim" || many[i].Text != " trim " {
					t.Fatal("batch identity or copy changed", i, item)
				}
			}
			after := sess.View()
			rawAfter, _ := json.Marshal(after[:len(before)])
			if string(rawAfter) != string(rawBefore) {
				t.Fatal("transform rewrote session history")
			}
		})
	}
}
