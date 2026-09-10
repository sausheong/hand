package app

import (
	"context"
	"encoding/json"
	"github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tools/bash"
	"github.com/sausheong/harness/tools/web"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResourcePeerProcess(t *testing.T) {
	mode := os.Getenv("HAND_RESOURCE_PEER")
	if mode == "" {
		return
	}
	method := os.Getenv("HAND_RESOURCE_METHOD")
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
			caps := []string{"commands", method}
			if mode == "no-capability" {
				caps = []string{"commands"}
			}
			reply.Result, _ = json.Marshal(protocol.Hello{Version: 1, Name: "resource-peer", Capabilities: caps, Commands: []protocol.Command{{Name: "execute", Description: "Execute fixture resource"}}})
		case "command.execute":
			if err = writer.Write(protocol.Frame{Version: 1, Kind: "request", ID: "resource-call", Method: method, Params: commandArguments(request.Params)}); err != nil {
				os.Exit(3)
			}
			response, err := reader.Read()
			if err != nil || response.ID != "resource-call" {
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

func TestExtensionResourceIPC(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"process.run", "network.fetch"} {
		for _, mode := range []string{"allow", "deny", "no-capability", "cancel", "public-fetch"} {
			if (method == "network.fetch" && mode == "cancel") || (method == "process.run" && mode == "public-fetch") {
				continue
			}
			t.Run(method+"/"+mode, func(t *testing.T) {
				publicURL := os.Getenv("HAND_TEST_PUBLIC_FETCH_URL")
				if mode == "public-fetch" && publicURL == "" {
					t.Skip("explicit public fetch fixture URL required")
				}
				ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
				defer cancel()
				workspace := t.TempDir()
				reg := tool.NewRegistry()
				reg.Register(&bash.BashTool{WorkDir: workspace})
				reg.Register(&web.WebFetchTool{})
				rt := &runtime.Runtime{Tools: reg}
				before, after := 0, 0
				rt.AgentLoop.Hooks.BeforeToolUse = func(_ context.Context, name string, _ json.RawMessage) (runtime.HookDecision, error) {
					before++
					expected := "bash"
					if method == "network.fetch" {
						expected = "web_fetch"
					}
					if name != expected {
						t.Fatal(name)
					}
					return runtime.HookDecision{Allow: mode == "allow" || mode == "cancel" || mode == "public-fetch"}, nil
				}
				rt.AgentLoop.Hooks.AfterToolUse = func(context.Context, string, json.RawMessage, tool.ToolResult) { after++ }
				c := &Controller{Rt: rt}
				caps := []string{"commands", method}
				if mode == "no-capability" {
					caps = []string{"commands"}
				}
				review, err := extensions.ReviewHostLaunch(ctx, extensions.LaunchConfig{Name: "resource-peer", Executable: exe, Workspace: workspace, Arguments: []string{"-test.run=^TestResourcePeerProcess$"}, Environment: map[string]string{"HAND_RESOURCE_PEER": mode, "HAND_RESOURCE_METHOD": method}, Capabilities: caps})
				if err != nil {
					t.Fatal(err)
				}
				snapshots := filepath.Join(t.TempDir(), "snapshots")
				host, err := c.ActivateExtensions(ctx, ExtensionStartup{Version: 1, SnapshotRoot: snapshots, Reviews: []extensions.LaunchReview{review}, Identities: map[string]string{"resource-peer": "fixture/resource-peer"}}, workspace, true)
				if err != nil {
					t.Fatal(err)
				}
				defer host.Close()
				args := `{"command":"printf ipc-command > marker.txt; cat marker.txt","timeout":5}`
				if method == "network.fetch" {
					args = `{"url":"http://127.0.0.1:1/private"}`
				}
				if mode == "public-fetch" {
					raw, _ := json.Marshal(map[string]string{"url": publicURL})
					args = string(raw)
				}
				if mode == "cancel" {
					callCtx, stop := context.WithTimeout(ctx, time.Second)
					start := time.Now()
					_, callErr := host.Execute(callCtx, "resource-peer", "execute", `{"command":"printf started > started.txt; sleep 20; printf late > marker.txt","timeout":30}`)
					stop()
					if callErr == nil || time.Since(start) > 5*time.Second {
						t.Fatal("active callback cancellation failed", callErr, time.Since(start))
					}
					if before != 1 {
						t.Fatal("command never reached approval", before)
					}
					started, readErr := os.ReadFile(filepath.Join(workspace, "started.txt"))
					if readErr != nil || string(started) != "started" {
						t.Fatal("command did not start before cancellation", string(started), readErr)
					}
					// Close joins connection and process cleanup, including the callback.
					_ = host.Close()
					if _, err := os.Stat(filepath.Join(workspace, "marker.txt")); !os.IsNotExist(err) {
						t.Fatal("cancelled command continued", err)
					}
					entries, err := os.ReadDir(snapshots)
					if err != nil || len(entries) != 0 {
						t.Fatal("cancelled launch snapshot leak", entries, err)
					}
					return
				}
				result, err := host.Execute(ctx, "resource-peer", "execute", args)
				if err != nil || len(result.Blocks) != 1 {
					t.Fatal(result, err)
				}
				switch mode {
				case "no-capability":
					if result.Blocks[0].Text != "capability_denied" || before != 0 || after != 0 {
						t.Fatal(result, before, after)
					}
				case "deny":
					if result.Blocks[0].Text != "permission_denied" || before != 1 || after != 0 {
						t.Fatal(result, before, after)
					}
				case "allow", "public-fetch":
					var output tool.ToolResult
					if err := json.Unmarshal([]byte(result.Blocks[0].Text), &output); err != nil {
						t.Fatal(err, result)
					}
					if before != 1 || after != 1 {
						t.Fatal(before, after)
					}
					if method == "process.run" {
						if output.Error != "" || !strings.Contains(output.Output, "ipc-command") {
							t.Fatal(output)
						}
					} else if mode == "public-fetch" {
						if output.Error != "" || strings.TrimSpace(output.Output) == "" || output.Metadata["url"] != publicURL || output.Metadata["status"] != float64(200) {
							t.Fatal("public fetch failed", output)
						}
					} else if output.Error == "" {
						t.Fatal("private network destination admitted", output)
					}
				}
				body, err := os.ReadFile(filepath.Join(workspace, "marker.txt"))
				if method == "process.run" && mode == "allow" {
					if err != nil || string(body) != "ipc-command" {
						t.Fatal(string(body), err)
					}
				} else if !os.IsNotExist(err) {
					t.Fatal("unexpected command execution", string(body), err)
				}
				if err := host.Close(); err != nil {
					t.Fatal(err)
				}
				entries, err := os.ReadDir(snapshots)
				if err != nil || len(entries) != 0 {
					t.Fatal("launch snapshot leak", entries, err)
				}
			})
		}
	}
}
