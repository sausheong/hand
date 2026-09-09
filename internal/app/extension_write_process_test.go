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
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
)

func TestFileWritePeerProcess(t *testing.T) {
	mode := os.Getenv("HAND_WRITE_PEER")
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
			caps := []string{"commands", "file.write"}
			if mode == "no-capability" {
				caps = []string{"commands"}
			}
			reply.Result, _ = json.Marshal(protocol.Hello{Version: 1, Name: "file-writer", Capabilities: caps, Commands: []protocol.Command{{Name: "write", Description: "Write fixture file"}}})
		case "command.execute":
			if err = writer.Write(protocol.Frame{Version: 1, Kind: "request", ID: "file-write", Method: "file.write", Params: commandArguments(request.Params)}); err != nil {
				os.Exit(3)
			}
			response, err := reader.Read()
			if err != nil || response.ID != "file-write" {
				os.Exit(4)
			}
			text := ""
			if response.Error != nil {
				text = response.Error.Code
			} else {
				text = string(response.Result)
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

func TestExtensionFileWriteProcessCapabilityAndHostApproval(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"allow", "deny", "no-capability", "policy-deny", "guidance"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			workspace := t.TempDir()
			if err := os.Mkdir(filepath.Join(workspace, "sub"), 0700); err != nil {
				t.Fatal(err)
			}
			if mode == "guidance" {
				if err := os.WriteFile(filepath.Join(workspace, "sub", "AGENTS.md"), []byte("Preserve fixture conventions"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err = os.WriteFile(filepath.Join(workspace, "sub", "note.txt"), []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			registry := tool.NewRegistry()
			registry.Register(agentio.WriteFileWithInstructions(workspace, true))
			rt := &runtime.Runtime{Tools: registry}
			rt.AgentLoop.Hooks.BeforeToolUse = func(context.Context, string, json.RawMessage) (runtime.HookDecision, error) {
				return runtime.HookDecision{Allow: mode == "allow" || mode == "policy-deny" || mode == "guidance"}, nil
			}
			c := &Controller{Rt: rt}
			caps := []string{"commands", "file.write"}
			if mode == "no-capability" {
				caps = []string{"commands"}
			}
			review, err := extensions.ReviewHostLaunch(ctx, extensions.LaunchConfig{Name: "file-writer", Executable: exe, Workspace: workspace, Arguments: []string{"-test.run=^TestFileWritePeerProcess$"}, Environment: map[string]string{"HAND_WRITE_PEER": mode}, Capabilities: caps})
			if err != nil {
				t.Fatal(err)
			}
			snapshots := filepath.Join(t.TempDir(), "snapshots")
			selected := ExtensionStartup{Version: 1, SnapshotRoot: snapshots, Identities: map[string]string{"file-writer": "fixture/file-writer"}, Reviews: []extensions.LaunchReview{review}}
			if mode == "policy-deny" {
				binary := filepath.Join(t.TempDir(), "policy")
				build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./examples/extensions/go-tool-policy")
				build.Dir = "../.."
				if out, err := build.CombinedOutput(); err != nil {
					t.Fatal(err, string(out))
				}
				policy, err := extensions.ReviewHostLaunch(ctx, extensions.LaunchConfig{Name: "tool-policy", Executable: binary, Workspace: workspace, Arguments: []string{"--deny-path", "sub/note.txt"}, Capabilities: []string{"policy.check"}, Mandatory: true})
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
			result, err := host.Execute(ctx, "file-writer", "write", `{"path":"sub/note.txt","content":"replacement"}`)
			if err != nil || len(result.Blocks) != 1 {
				t.Fatal(result, err)
			}
			if mode == "guidance" {
				var guidance tool.ToolResult
				if err := json.Unmarshal([]byte(result.Blocks[0].Text), &guidance); err != nil || guidance.Error == "" {
					t.Fatal(result, err)
				}
				body, err := os.ReadFile(filepath.Join(workspace, "sub", "note.txt"))
				if err != nil || string(body) != "original" {
					t.Fatal("guidance response mutated file", string(body), err)
				}
				digest, ok := guidance.Metadata["instruction_digest"].(string)
				if !ok || len(digest) != 64 {
					t.Fatal("missing digest", guidance)
				}
				args, _ := json.Marshal(map[string]string{"path": "sub/note.txt", "content": "replacement", "instruction_digest": digest})
				result, err = host.Execute(ctx, "file-writer", "write", string(args))
				if err != nil || len(result.Blocks) != 1 {
					t.Fatal(result, err)
				}
			}
			expected := map[string]string{"deny": "permission_denied", "no-capability": "capability_denied", "policy-deny": "permission_denied"}[mode]
			if mode == "allow" || mode == "guidance" {
				var written tool.ToolResult
				if err := json.Unmarshal([]byte(result.Blocks[0].Text), &written); err != nil || written.Error != "" {
					t.Fatal(result, err)
				}
			} else if result.Blocks[0].Text != expected {
				t.Fatal(result, expected)
			}
			body, err := os.ReadFile(filepath.Join(workspace, "sub", "note.txt"))
			want := "original"
			if mode == "allow" || mode == "guidance" {
				want = "replacement"
			}
			if err != nil || string(body) != want {
				t.Fatal("unexpected file contents", string(body), err)
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

// The extension forwards its command argument as a resource request; the host
// must still validate and authorise it independently.
func commandArguments(raw json.RawMessage) json.RawMessage {
	var command struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &command); err != nil {
		os.Exit(8)
	}
	return json.RawMessage(command.Arguments)
}
