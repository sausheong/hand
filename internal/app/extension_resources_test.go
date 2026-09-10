package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tools/file"
)

func TestExtensionFileReadUsesHostApprovalAndRootedTool(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("workspace content"), 0600); err != nil {
		t.Fatal(err)
	}
	reg := tool.NewRegistry()
	reg.Register(&file.ReadFileTool{WorkDir: workspace, ExactPath: true})
	allowed := false
	before, after := 0, 0
	rt := &runtime.Runtime{Tools: reg}
	rt.AgentLoop.Hooks.BeforeToolUse = func(ctx context.Context, name string, raw json.RawMessage) (runtime.HookDecision, error) {
		before++
		if name != "read_file" {
			t.Fatal(name)
		}
		return runtime.HookDecision{Allow: allowed}, nil
	}
	rt.AgentLoop.Hooks.AfterToolUse = func(context.Context, string, json.RawMessage, tool.ToolResult) { after++ }
	host := &ExtensionHost{controller: &Controller{Rt: rt}}
	spec := extensions.Specification{Name: "reader", Capabilities: []string{"file.read"}}
	input := json.RawMessage(`{"path":"note.txt"}`)
	if raw, e := host.readExtensionFile(context.Background(), spec, input); e == nil || len(raw) != 0 || after != 0 {
		t.Fatal("denied read executed", string(raw), e, after)
	}
	allowed = true
	raw, e := host.readExtensionFile(context.Background(), spec, input)
	if e != nil || !strings.Contains(string(raw), "workspace content") || before != 2 || after != 1 {
		t.Fatal(string(raw), e, before, after)
	}
	raw, e = host.readExtensionFile(context.Background(), spec, json.RawMessage(`{"path":"../outside"}`))
	if e == nil && !strings.Contains(string(raw), `"error"`) {
		t.Fatal("workspace escape accepted", string(raw))
	}
	prior := before
	spec.Capabilities = append(spec.Capabilities, "policy.check")
	if _, e = host.readExtensionFile(context.Background(), spec, input); e == nil || before != prior {
		t.Fatal("recursive policy read admitted")
	}
	spec.Capabilities = []string{"file.read"}
	if _, e = host.readExtensionFile(context.WithValue(context.Background(), extensionPolicyContextKey{}, true), spec, input); e == nil {
		t.Fatal("policy callback recursion admitted")
	}
	if _, e = host.readExtensionFile(context.Background(), spec, json.RawMessage(`{"path":"note.txt","write":true}`)); e == nil {
		t.Fatal("unknown input accepted")
	}
	rt.AgentLoop.Hooks.BeforeToolUse = func(context.Context, string, json.RawMessage) (runtime.HookDecision, error) { panic("hook failure") }
	if _, e = host.readExtensionFile(context.Background(), spec, input); e == nil {
		t.Fatal("hook panic not contained")
	}
}

func TestExtensionFileWriteApprovalGuidanceAndUncertainty(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	guidance := filepath.Join(workspace, "sub", "AGENTS.md")
	if err := os.WriteFile(guidance, []byte("First constraint"), 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(workspace, "sub", "note.txt")
	reg := tool.NewRegistry()
	reg.Register(agentio.WriteFileWithInstructions(workspace, true))
	rt := &runtime.Runtime{Tools: reg}
	allowed := false
	rt.AgentLoop.Hooks.BeforeToolUse = func(_ context.Context, name string, _ json.RawMessage) (runtime.HookDecision, error) {
		if name != "write_file" {
			t.Fatal(name)
		}
		return runtime.HookDecision{Allow: allowed}, nil
	}
	host := &ExtensionHost{controller: &Controller{Rt: rt}}
	spec := extensions.Specification{Name: "writer", Capabilities: []string{"file.write"}}
	input := map[string]any{"path": "sub/note.txt", "content": "first"}
	call := func() (tool.ToolResult, string) {
		t.Helper()
		raw, _ := json.Marshal(input)
		response, e := host.extensionFileResource(context.Background(), spec, "file.write", raw)
		if e != nil {
			return tool.ToolResult{}, e.Code
		}
		var result tool.ToolResult
		if err := json.Unmarshal(response, &result); err != nil {
			t.Fatal(err)
		}
		return result, ""
	}
	absent := func() {
		t.Helper()
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatal("unexpected mutation", err)
		}
	}
	if _, code := call(); code != "permission_denied" {
		t.Fatal(code)
	}
	absent()
	allowed = true
	result, code := call()
	if code != "" || result.Error == "" || !strings.Contains(result.Output, "First constraint") {
		t.Fatal(result, code)
	}
	absent()
	digest, _ := result.Metadata["instruction_digest"].(string)
	if len(digest) != 64 {
		t.Fatal("missing instruction digest", result)
	}
	input["instruction_digest"] = digest
	if err := os.WriteFile(guidance, []byte("Changed constraint"), 0600); err != nil {
		t.Fatal(err)
	}
	result, code = call()
	if code != "" || result.Error == "" || result.Metadata["instruction_digest"] == digest {
		t.Fatal("stale guidance accepted", result, code)
	}
	absent()
	input["instruction_digest"] = result.Metadata["instruction_digest"]
	result, code = call()
	if code != "" || result.Error != "" {
		t.Fatal(result, code)
	}
	body, err := os.ReadFile(target)
	if err != nil || string(body) != "first" {
		t.Fatal(string(body), err)
	}
	input["content"] = "second"
	rt.AgentLoop.Hooks.AfterToolUse = func(context.Context, string, json.RawMessage, tool.ToolResult) { panic("observer failure") }
	if _, code = call(); code != "mutation_outcome_unknown" {
		t.Fatal("committed write reported as retryable failure", code)
	}
	body, err = os.ReadFile(target)
	if err != nil || string(body) != "second" {
		t.Fatal(string(body), err)
	}
}
