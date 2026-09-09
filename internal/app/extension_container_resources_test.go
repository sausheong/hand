package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/hand/internal/isolation"
	"github.com/sausheong/harness/execution"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
)

func TestContainerExtensionFileCallbackBoundary(t *testing.T) {
	image, socket := os.Getenv("HARNESS_TEST_CONTAINER_IMAGE"), os.Getenv("HARNESS_TEST_CONTAINER_SOCKET")
	peer, worker := os.Getenv("HAND_TEST_EXTENSION_PEER_BINARY"), os.Getenv("HAND_TEST_WORKER_BINARY")
	if image == "" || socket == "" || peer == "" || worker == "" {
		t.Skip("native container peer and worker fixtures required")
	}
	workerBytes, err := os.ReadFile(worker)
	if err != nil {
		t.Fatal(err)
	}
	workerHash := sha256.Sum256(workerBytes)
	for _, mode := range []string{"read-only", "writable", "denied"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			workspace := t.TempDir()
			target := filepath.Join(workspace, "note.txt")
			if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			backend := execution.Container{Docker: "/usr/local/bin/docker", Socket: socket, Image: image, Workspace: workspace, Writable: mode != "read-only", WorkerPath: worker, WorkerSHA256: hex.EncodeToString(workerHash[:])}
			reg := tool.NewRegistry()
			reg.Register(agentio.WriteFileWithInstructions(workspace, true))
			if err := isolation.Install(reg, backend, workspace); err != nil {
				t.Fatal(err)
			}
			rt := &runtime.Runtime{Tools: reg}
			rt.AgentLoop.Hooks.BeforeToolUse = func(_ context.Context, name string, _ json.RawMessage) (runtime.HookDecision, error) {
				if name != "write_file" {
					t.Fatal(name)
				}
				return runtime.HookDecision{Allow: mode != "denied"}, nil
			}
			review, err := extensions.ReviewContainerLaunch(ctx, extensions.LaunchConfig{Name: "file-writer", Executable: peer, Workspace: workspace, Arguments: []string{"-test.run=^TestFileWritePeerProcess$"}, Environment: map[string]string{"HAND_WRITE_PEER": "allow"}, Capabilities: []string{"commands", "file.write"}}, extensions.ContainerLaunch{Docker: backend.Docker, Socket: socket, Image: image, Writable: backend.Writable})
			if err != nil {
				t.Fatal(err)
			}
			c := &Controller{Rt: rt}
			host, err := c.ActivateExtensionsWithBackend(ctx, ExtensionStartup{Version: 1, SnapshotRoot: filepath.Join(t.TempDir(), "snapshots"), Identities: map[string]string{"file-writer": "fixture/writer"}, Reviews: []extensions.LaunchReview{review}}, workspace, backend)
			if err != nil {
				t.Fatal(err)
			}
			defer host.Close()
			response, err := host.Execute(ctx, "file-writer", "write", `{"path":"note.txt","content":"container callback"}`)
			if err != nil || len(response.Blocks) != 1 {
				t.Fatal(response, err)
			}
			if mode == "denied" {
				if response.Blocks[0].Text != "permission_denied" {
					t.Fatal(response)
				}
			} else {
				var result tool.ToolResult
				if err := json.Unmarshal([]byte(response.Blocks[0].Text), &result); err != nil {
					t.Fatal(response, err)
				}
				if mode == "writable" && result.Error != "" {
					t.Fatal(result)
				}
				if mode == "read-only" && result.Error == "" {
					t.Fatal("read-only boundary bypassed", result)
				}
			}
			body, err := os.ReadFile(target)
			want := "original"
			if mode == "writable" {
				want = "container callback"
			}
			if err != nil || string(body) != want {
				t.Fatal(string(body), err)
			}
			if err = host.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
