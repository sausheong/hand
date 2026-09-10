package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
)

type resourceFailureTool struct {
	name string
	run  func() (tool.ToolResult, error)
}

func (t resourceFailureTool) Name() string                           { return t.name }
func (t resourceFailureTool) IsConcurrencySafe(json.RawMessage) bool { return false }
func (t resourceFailureTool) Description() string                    { return "resource failure fixture" }
func (t resourceFailureTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (t resourceFailureTool) Execute(context.Context, json.RawMessage) (tool.ToolResult, error) {
	return t.run()
}

func TestExtensionResourceFailurePreservesMutationUncertainty(t *testing.T) {
	for _, method := range []struct {
		name, tool, input string
		mutates           bool
	}{
		{"file.read", "read_file", `{"path":"note"}`, false},
		{"file.write", "write_file", `{"path":"note","content":"value"}`, true},
		{"process.run", "bash", `{"command":"fixture"}`, true},
		{"network.fetch", "web_fetch", `{"url":"https://example.com"}`, true},
	} {
		for _, failure := range []string{"error", "output-limit", "error-limit", "encoding-error", "encoded-limit", "panic"} {
			t.Run(method.name+"/"+failure, func(t *testing.T) {
				marker := filepath.Join(t.TempDir(), "effect")
				calls, after := 0, 0
				reg := tool.NewRegistry()
				reg.Register(resourceFailureTool{name: method.tool, run: func() (tool.ToolResult, error) {
					calls++
					if method.mutates {
						if err := os.WriteFile(marker, []byte("applied"), 0600); err != nil {
							t.Fatal(err)
						}
					}
					switch failure {
					case "error":
						return tool.ToolResult{}, errors.New("fixture")
					case "output-limit":
						return tool.ToolResult{Output: strings.Repeat("x", (64<<10)+1)}, nil
					case "error-limit":
						return tool.ToolResult{Error: strings.Repeat("x", 4097)}, nil
					case "encoding-error":
						return tool.ToolResult{Metadata: map[string]any{"unsupported": make(chan int)}}, nil
					case "encoded-limit":
						return tool.ToolResult{Output: strings.Repeat("\x00", 32<<10)}, nil
					default:
						panic("after effect")
					}
				}})
				rt := &runtime.Runtime{Tools: reg}
				rt.AgentLoop.Hooks.AfterToolUse = func(context.Context, string, json.RawMessage, tool.ToolResult) { after++ }
				host := &ExtensionHost{controller: &Controller{Rt: rt}}
				raw, e := host.extensionToolResource(context.Background(), extensions.Specification{Name: "failure", Capabilities: []string{method.name}}, method.name, json.RawMessage(method.input))
				if e == nil || len(raw) != 0 || calls != 1 {
					t.Fatalf("raw=%s error=%v calls=%d", raw, e, calls)
				}
				expected := "resource_limit"
				if failure == "error" || failure == "encoding-error" || failure == "panic" {
					expected = "resource_failed"
				}
				if method.mutates {
					expected = "mutation_outcome_unknown"
				}
				if e.Code != expected {
					t.Fatalf("got %s want %s", e.Code, expected)
				}
				if method.mutates {
					if b, err := os.ReadFile(marker); err != nil || string(b) != "applied" {
						t.Fatalf("lost effect: %s %v", b, err)
					}
				}
				if failure != "panic" && after != 1 {
					t.Fatalf("after hook count=%d", after)
				}
			})
		}
	}
}
