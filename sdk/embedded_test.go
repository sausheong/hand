//go:build darwin || linux

package sdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sausheong/hand/protocol"
	"github.com/sausheong/hand/sdk"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEmbeddedSessionLifecycleAndRelease(t *testing.T) {
	opts := sdk.EmbeddedOptions{Workspace: t.TempDir(), StoreDirectory: t.TempDir(), Model: "local/test", AuthorityDirectory: filepath.Join(t.TempDir(), "authority")}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := sdk.Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err = client.Hello(ctx, "hello"); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Call(ctx, "permissions", "permission.list", nil); err != nil {
		t.Fatal(err)
	}
	raw, err := client.Call(ctx, "sessions", "session.list", nil)
	if err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Total int `json:"total"`
	}
	if err = json.Unmarshal(raw, &listed); err != nil || listed.Total != 1 {
		t.Fatalf("sessions %s %v", raw, err)
	}
	if _, err = client.Call(ctx, "new", "session.new", nil); err != nil {
		t.Fatal(err)
	}
	if err = client.Close(); err != nil {
		t.Fatal(err)
	}
	// Reopen the durable store after closing the current and previous sessions.
	again, err := sdk.Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if _, err = again.Hello(ctx, "hello2"); err != nil {
		t.Fatal(err)
	}
	if _, err = again.Call(ctx, "new", "session.new", nil); err != nil {
		t.Fatal(err)
	}
	raw, err = again.Call(ctx, "sessions2", "session.list", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &listed); err != nil || listed.Total != 2 {
		t.Fatalf("reopened sessions %s %v", raw, err)
	}
	if err = again.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestEmbeddedCancelledConstruction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if c, err := sdk.Open(ctx, sdk.EmbeddedOptions{}); err == nil || c != nil {
		t.Fatal("cancelled construction accepted")
	}
}

func TestEmbeddedPromptUsesRealRuntime(t *testing.T) {
	seen := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		for _, m := range body.Messages {
			if m.Role == "user" {
				seen <- string(m.Content)
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"index":0,"delta":{"content":"embedded answer"},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	options := sdk.EmbeddedOptions{Workspace: t.TempDir(), StoreDirectory: t.TempDir(), Model: "local/fixture", Endpoint: server.URL + "/v1", CheckpointDirectory: filepath.Join(t.TempDir(), "checkpoints")}
	options.Checkpoints = sdk.CheckpointOptions{Exclude: []string{"private"}, MaxEntries: 10, MaxFileBytes: 16, MaxTotalBytes: 32, MaxSnapshots: 1, MaxStoreBytes: 1 << 20}
	if err := os.Mkdir(filepath.Join(options.Workspace, "private"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(options.Workspace, "private", "token"), []byte(strings.Repeat("private payload", 100)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(options.Workspace, "source"), []byte("code"), 0600); err != nil {
		t.Fatal(err)
	}
	evidence := t.TempDir()
	if err := os.Chmod(evidence, 0700); err != nil {
		t.Fatal(err)
	}
	options.Verification = &sdk.VerificationOptions{Directory: evidence, Profiles: []sdk.VerificationProfile{{Name: "unit", Command: []string{"go", "test", "./..."}}}}
	c, err := sdk.Open(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	options.Checkpoints.Exclude[0] = "different" // The active boundary owns its copy.
	if _, err = c.Hello(ctx, "h"); err != nil {
		t.Fatal(err)
	}
	profilesRaw, err := c.Call(ctx, "verification-profiles", "verification.profiles", nil)
	if err != nil {
		t.Fatal(err)
	}
	var profiles []struct {
		Name    string
		Command []string
		Digest  string
	}
	if err = json.Unmarshal(profilesRaw, &profiles); err != nil || len(profiles) != 1 || profiles[0].Name != "unit" || len(profiles[0].Digest) != 64 || profiles[0].Command[0] != "go" {
		t.Fatal(string(profilesRaw), err)
	}
	if _, err = c.Prompt(ctx, "p", "verify embedded prompt"); err != nil {
		t.Fatal(err)
	}
	for {
		raw, e := c.Call(ctx, "get", "request.get", map[string]string{"id": "p"})
		if e != nil {
			t.Fatal(e)
		}
		var record struct {
			State  string
			Result protocol.Event
		}
		if e = json.Unmarshal(raw, &record); e != nil {
			t.Fatal(e)
		}
		if record.State == "completed" {
			if record.Result.Kind != "terminal" || !strings.Contains(string(record.Result.Payload), `"status":"completed"`) {
				t.Fatalf("terminal %s", raw)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("run did not finish")
		case <-time.After(time.Millisecond):
		}
	}
	select {
	case prompt := <-seen:
		if !strings.Contains(prompt, "verify embedded prompt") {
			t.Fatal(prompt)
		}
	default:
		t.Fatal("provider request absent")
	}

	if _, err = c.Call(ctx, "new-after-prompt", "session.new", nil); err != nil {
		t.Fatal(err)
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	snapshots, globErr := filepath.Glob(filepath.Join(options.CheckpointDirectory, "*.json"))
	if globErr != nil || len(snapshots) != 1 {
		t.Fatal("checkpoint not persisted", snapshots, globErr)
	}
	rawSnapshot, readErr := os.ReadFile(snapshots[0])
	if readErr != nil {
		t.Fatal(readErr)
	}
	var savedSnapshot struct {
		Digest  string
		Records []struct {
			Path string `json:"path"`
		}
		Omissions []struct {
			Path string `json:"path"`
		}
		Content map[string][]byte
	}
	if err = json.Unmarshal(rawSnapshot, &savedSnapshot); err != nil || savedSnapshot.Digest+".json" != filepath.Base(snapshots[0]) {
		t.Fatal("checkpoint identity mismatch", err)
	}
	if len(savedSnapshot.Records) != 1 || savedSnapshot.Records[0].Path != "source" || len(savedSnapshot.Content) != 1 {
		t.Fatal("configured exclusion ignored", string(rawSnapshot))
	}
	for _, content := range savedSnapshot.Content {
		if string(content) != "code" {
			t.Fatal("excluded bytes persisted")
		}
	}
	options.Checkpoints.Exclude[0] = "private"
	reopened, err := sdk.Open(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err = reopened.Hello(ctx, "reconnect"); err != nil {
		t.Fatal(err)
	}
	raw, err := reopened.Call(ctx, "old-result", "request.get", map[string]string{"id": "p"})
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		State  string
		Result protocol.Event
	}
	if err = json.Unmarshal(raw, &record); err != nil || record.State != "completed" || record.Result.Kind != "terminal" {
		t.Fatalf("lost execution across session switch: %s %v", raw, err)
	}
	if _, err = reopened.Prompt(ctx, "p", "verify embedded prompt"); err == nil {
		t.Fatal("same ID in a different session must conflict, not execute")
	}
	select {
	case extra := <-seen:
		t.Fatalf("provider replayed request: %s", extra)
	default:
	}
	if err = reopened.Close(); err != nil {
		t.Fatal(err)
	}

}

func TestEmbeddedCheckpointRejectsWorkspaceDirectory(t *testing.T) {
	workspace := t.TempDir()
	options := sdk.EmbeddedOptions{Workspace: workspace, StoreDirectory: t.TempDir(), Model: "local/fixture", CheckpointDirectory: filepath.Join(workspace, "checkpoints")}
	if c, err := sdk.Open(context.Background(), options); err == nil {
		c.Close()
		t.Fatal("workspace checkpoint storage accepted")
	}
	if _, err := os.Stat(options.CheckpointDirectory); !os.IsNotExist(err) {
		t.Fatal("rejected checkpoint created workspace state", err)
	}
}

func TestEmbeddedCheckpointOptionsRejectBeforeCreatingResources(t *testing.T) {
	for _, config := range []sdk.CheckpointOptions{{MaxEntries: -1}, {Exclude: []string{"../outside"}}, {MaxStoreBytes: 9 << 30}} {
		root := t.TempDir()
		options := sdk.EmbeddedOptions{Workspace: root, StoreDirectory: filepath.Join(root, "sessions"), CheckpointDirectory: filepath.Join(t.TempDir(), "checkpoints"), Model: "local/fixture", Checkpoints: config}
		if c, err := sdk.Open(context.Background(), options); err == nil {
			c.Close()
			t.Fatal("invalid options accepted")
		}
		for _, path := range []string{options.StoreDirectory, options.CheckpointDirectory} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("invalid options created resources", path, err)
			}
		}
	}
}
