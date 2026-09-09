package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tools/bash"
	"github.com/sausheong/harness/tools/web"
)

func TestExtensionNetworkAndProcessRegisteredTools(t *testing.T) {
	workspace := t.TempDir()
	reg := tool.NewRegistry()
	reg.Register(&bash.BashTool{WorkDir: workspace})
	reg.Register(&web.WebFetchTool{})
	rt := &runtime.Runtime{Tools: reg}
	allowed := false
	observed := ""
	rt.AgentLoop.Hooks.BeforeToolUse = func(_ context.Context, name string, _ json.RawMessage) (runtime.HookDecision, error) {
		observed = name
		return runtime.HookDecision{Allow: allowed}, nil
	}
	host := &ExtensionHost{controller: &Controller{Rt: rt}}
	spec := extensions.Specification{Name: "resources", Capabilities: []string{"process.run", "network.fetch"}}
	run := func(ctx context.Context, method, input string) (tool.ToolResult, string) {
		t.Helper()
		raw, e := host.extensionToolResource(ctx, spec, method, json.RawMessage(input))
		if e != nil {
			return tool.ToolResult{}, e.Code
		}
		var result tool.ToolResult
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		return result, ""
	}
	command := `{"command":"printf callback-result > result.txt; cat result.txt","timeout":5}`
	if _, code := run(context.Background(), "process.run", command); code != "permission_denied" || observed != "bash" {
		t.Fatal(code, observed)
	}
	if _, err := os.Stat(filepath.Join(workspace, "result.txt")); !os.IsNotExist(err) {
		t.Fatal("denied command ran", err)
	}
	allowed = true
	result, code := run(context.Background(), "process.run", command)
	if code != "" || result.Error != "" || !strings.Contains(result.Output, "callback-result") {
		t.Fatal(result, code)
	}
	body, err := os.ReadFile(filepath.Join(workspace, "result.txt"))
	if err != nil || string(body) != "callback-result" {
		t.Fatal(string(body), err)
	}
	result, code = run(context.Background(), "network.fetch", `{"url":"http://127.0.0.1:1/private"}`)
	if code != "" || result.Error == "" || observed != "web_fetch" {
		t.Fatal("private address accepted", result, code, observed)
	}
	allowed = false
	if _, code = run(context.Background(), "network.fetch", `{"url":"https://example.com"}`); code != "permission_denied" {
		t.Fatal(code)
	}
	for _, tc := range []struct{ method, input string }{
		{"network.fetch", `{"url":"file:///etc/passwd"}`},
		{"network.fetch", `{"url":"https://user:secret@example.com"}`},
		{"network.fetch", `{"url":"https://example.com","headers":{"X-Test":"a\r\nb"}}`},
		{"network.fetch", `{"url":"https://example.com","method":"POST"}`},
		{"process.run", `{"command":"true","timeout":121}`},
		{"process.run", `{"command":"true","env":{"X":"y"}}`},
	} {
		observed = ""
		if _, code = run(context.Background(), tc.method, tc.input); code != "invalid_input" || observed != "" {
			t.Fatal(tc, code, observed)
		}
	}
	allowed = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, code = run(ctx, "process.run", `{"command":"touch cancelled.txt"}`); code != "cancelled" {
		t.Fatal(code)
	}
	if _, err := os.Stat(filepath.Join(workspace, "cancelled.txt")); !os.IsNotExist(err) {
		t.Fatal("cancelled command ran", err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _ = run(ctx, "process.run", `{"command":"sleep 20","timeout":30}`)
	if ctx.Err() == nil || time.Since(start) > 5*time.Second {
		t.Fatal("command cancellation did not join promptly", ctx.Err(), time.Since(start))
	}
}
