package agentio_test

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tools/mcp"
)

var (
	mcpServerFixtureOnce sync.Once
	mcpServerFixturePath string
	mcpServerFixtureErr  error
)

// buildMCPServerFixture compiles internal/agentio/testdata/mcpserver (a
// minimal real MCP stdio server, see that file) into a temp binary once
// per test run, so BuildRuntimeWithTimeout's "normal, quickly-resolving
// call" case exercises a real mcp.Connect handshake rather than a faked
// one.
func buildMCPServerFixture(t *testing.T) string {
	t.Helper()
	mcpServerFixtureOnce.Do(func() {
		dir := t.TempDir()
		out := filepath.Join(dir, "mcpserver")
		cmd := exec.Command("go", "build", "-o", out, "./testdata/mcpserver")
		if output, err := cmd.CombinedOutput(); err != nil {
			mcpServerFixtureErr = fmt.Errorf("build mcpserver fixture: %w: %s", err, output)
			return
		}
		mcpServerFixturePath = out
	})
	if mcpServerFixtureErr != nil {
		t.Fatalf("%v", mcpServerFixtureErr)
	}
	return mcpServerFixturePath
}

func TestBuildRuntimeWithTimeout_ZeroServersSkipsTimeoutPath(t *testing.T) {
	spec := runtime.AgentSpec{ID: "a", Name: "A", Model: "anthropic/claude-sonnet-5"}

	// timeout=0 would fire time.After(0) almost immediately if
	// BuildRuntimeWithTimeout mistakenly always took the goroutine+select
	// path — a real BuildRuntime call, even for zero MCP servers, needs a
	// goroutine spawn plus a channel round trip, so it would almost
	// certainly lose that race. Succeeding here demonstrates the
	// zero-MCPServers case bypasses the select path entirely.
	rt, err := agentio.BuildRuntimeWithTimeout(runtime.RuntimeDeps{}, runtime.RuntimeInputs{}, spec, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt == nil {
		t.Fatal("expected a non-nil Runtime")
	}
}

func TestBuildRuntimeWithTimeout_NormalCallSucceedsUnderShortTimeout(t *testing.T) {
	binPath := buildMCPServerFixture(t)
	reg := tool.NewRegistry()
	spec := runtime.AgentSpec{
		ID: "a", Name: "A", Model: "anthropic/claude-sonnet-5",
		MCPServers: []mcp.ServerConfig{{Name: "fixture", Command: binPath}},
	}

	// 2s (not the spec's illustrative 100ms) — spawning a real OS process
	// and completing the MCP initialize handshake over stdio pipes is
	// slower than an in-process fake, especially under a loaded/sandboxed
	// test runner; still tiny next to DefaultMCPConnectTimeout's 15s.
	rt, err := agentio.BuildRuntimeWithTimeout(runtime.RuntimeDeps{}, runtime.RuntimeInputs{Tools: reg}, spec, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt == nil {
		t.Fatal("expected a non-nil Runtime")
	}
	if err := rt.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestBuildRuntimeWithTimeout_HangingServerTimesOutFast(t *testing.T) {
	reg := tool.NewRegistry()
	spec := runtime.AgentSpec{
		ID: "a", Name: "A", Model: "anthropic/claude-sonnet-5",
		MCPServers: []mcp.ServerConfig{{Name: "hangs", Command: "sleep", Args: []string{"30"}}},
	}

	const timeout = 50 * time.Millisecond
	start := time.Now()
	rt, err := agentio.BuildRuntimeWithTimeout(runtime.RuntimeDeps{}, runtime.RuntimeInputs{Tools: reg}, spec, timeout)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if rt != nil {
		t.Fatal("expected a nil Runtime on timeout")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("BuildRuntimeWithTimeout took %s, want approximately the %s test timeout, never the real DefaultMCPConnectTimeout", elapsed, timeout)
	}
}
