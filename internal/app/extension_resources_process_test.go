package app

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tools/file"
)

func TestFileReadPeerProcess(t *testing.T) {
	mode := os.Getenv("HAND_READ_PEER")
	if mode == "" {
		return
	}
	reader, writer := protocol.NewReader(os.Stdin), protocol.NewWriter(os.Stdout)
	for {
		request, err := reader.Read()
		if err == io.EOF {
			os.Exit(0)
		}
		if err != nil {
			os.Exit(2)
		}
		reply := protocol.Frame{Version: 1, Kind: "response", ID: request.ID}
		switch request.Method {
		case "initialize":
			caps := []string{"commands", "file.read"}
			if mode == "no-capability" {
				caps = []string{"commands"}
			}
			reply.Result, _ = json.Marshal(protocol.Hello{Version: 1, Name: "file-reader", Capabilities: caps, Commands: []protocol.Command{{Name: "read", Description: "Read fixture file"}}})
		case "command.execute":
			if err = writer.Write(protocol.Frame{Version: 1, Kind: "request", ID: "file-read", Method: "file.read", Params: json.RawMessage(`{"path":"note.txt"}`)}); err != nil {
				os.Exit(3)
			}
			response, err := reader.Read()
			if err != nil || response.ID != "file-read" {
				os.Exit(4)
			}
			text := ""
			if response.Error != nil {
				text = response.Error.Code
			} else {
				var result struct {
					Output string `json:"output"`
					Error  string `json:"error"`
				}
				if err = json.Unmarshal(response.Result, &result); err != nil {
					os.Exit(5)
				}
				text = result.Output
				if result.Error != "" {
					text = result.Error
				}
			}
			reply.Result, _ = json.Marshal(protocol.Presentation{Blocks: []protocol.Block{{Kind: "text", Text: text}}})
		default:
			os.Exit(6)
		}
		if err = writer.Write(reply); err != nil {
			os.Exit(7)
		}
	}
}

func TestExtensionFileReadProcessCapabilityAndHostApproval(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"allow", "deny", "no-capability", "policy-deny"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			workspace := t.TempDir()
			if err = os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("approved workspace content"), 0600); err != nil {
				t.Fatal(err)
			}
			registry := tool.NewRegistry()
			registry.Register(&file.ReadFileTool{WorkDir: workspace, ExactPath: true})
			rt := &runtime.Runtime{Tools: registry}
			rt.AgentLoop.Hooks.BeforeToolUse = func(context.Context, string, json.RawMessage) (runtime.HookDecision, error) {
				return runtime.HookDecision{Allow: mode == "allow" || mode == "policy-deny"}, nil
			}
			c := &Controller{Rt: rt}
			caps := []string{"commands", "file.read"}
			if mode == "no-capability" {
				caps = []string{"commands"}
			}
			review, err := extensions.ReviewHostLaunch(ctx, extensions.LaunchConfig{Name: "file-reader", Executable: exe, Workspace: workspace, Arguments: []string{"-test.run=^TestFileReadPeerProcess$"}, Environment: map[string]string{"HAND_READ_PEER": mode}, Capabilities: caps})
			if err != nil {
				t.Fatal(err)
			}
			snapshots := filepath.Join(t.TempDir(), "snapshots")
			selected := ExtensionStartup{Version: 1, SnapshotRoot: snapshots, Identities: map[string]string{"file-reader": "fixture/file-reader"}, Reviews: []extensions.LaunchReview{review}}
			if mode == "policy-deny" {
				binary := filepath.Join(t.TempDir(), "policy")
				build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./examples/extensions/go-tool-policy")
				build.Dir = "../.."
				if out, err := build.CombinedOutput(); err != nil {
					t.Fatal(err, string(out))
				}
				policy, err := extensions.ReviewHostLaunch(ctx, extensions.LaunchConfig{Name: "tool-policy", Executable: binary, Workspace: workspace, Arguments: []string{"--deny-path", "note.txt"}, Capabilities: []string{"policy.check"}, Mandatory: true})
				if err != nil {
					t.Fatal(err)
				}
				selected.Reviews = append(selected.Reviews, policy)
				selected.Identities["tool-policy"] = "fixture/policy"
			}
			host, err := c.ActivateExtensions(ctx, selected, workspace, true)
			if err != nil {
				t.Fatal(err)
			}
			defer host.Close()
			result, err := host.Execute(ctx, "file-reader", "read", "")
			if err != nil || len(result.Blocks) != 1 {
				t.Fatal(result, err)
			}
			expected := map[string]string{"allow": "approved workspace content", "deny": "permission_denied", "no-capability": "capability_denied", "policy-deny": "permission_denied"}[mode]
			if result.Blocks[0].Text != expected {
				t.Fatal(result, expected)
			}
			if err = host.Close(); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(snapshots)
			if err != nil || len(entries) != 0 {
				t.Fatal("snapshot leaked", entries, err)
			}
		})
	}
}
