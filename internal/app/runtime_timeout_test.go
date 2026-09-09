package app_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tools/mcp"
)

// Compile a real stdio peer. Each test owns its fixture directory and cleanup.
func buildMCPServerFixture(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "mcpserver")
	cmd := exec.Command("go", "build", "-o", out, "../agentio/testdata/mcpserver")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build MCP fixture: %v: %s", err, output)
	}
	return out
}

func buildRuntimeWithTimeout(t *testing.T, deps runtime.RuntimeDeps, inputs runtime.RuntimeInputs, spec runtime.AgentSpec, timeout time.Duration) (*runtime.Runtime, error) {
	t.Helper()
	construction, err := app.NewRuntimeConstruction(deps, inputs, spec, false)
	if err != nil {
		return nil, err
	}
	return construction.BuildWithTimeout(context.Background(), timeout)
}

func TestBuildRuntimeWithTimeout_ZeroServersSkipsTimeoutPath(t *testing.T) {
	spec := runtime.AgentSpec{ID: "a", Name: "A", Model: "anthropic/claude-sonnet-5"}

	// No-server startup ignores the MCP-specific deadline.
	rt, err := buildRuntimeWithTimeout(t, runtime.RuntimeDeps{}, runtime.RuntimeInputs{}, spec, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt == nil {
		t.Fatal("expected a non-nil Runtime")
	}
	defer rt.Close()
}

func TestBuildRuntimeWithTimeout_NormalCallSucceedsUnderShortTimeout(t *testing.T) {
	binPath := buildMCPServerFixture(t)
	reg := tool.NewRegistry()
	spec := runtime.AgentSpec{
		ID: "a", Name: "A", Model: "anthropic/claude-sonnet-5",
		MCPServers: []mcp.ServerConfig{{Name: "fixture", Command: binPath}},
	}

	// Use the configured startup deadline for real child initialisation.
	// The hanging-server test below independently checks short timeout latency.
	rt, err := buildRuntimeWithTimeout(t, runtime.RuntimeDeps{}, runtime.RuntimeInputs{Tools: reg}, spec, app.DefaultMCPConnectTimeout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt == nil {
		t.Fatal("expected a non-nil Runtime")
	}
	if names := reg.Names(); len(names) != 1 || names[0] != "mcp__fixture__echo" {
		t.Fatalf("catalogue: %v", names)
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
	rt, err := buildRuntimeWithTimeout(t, runtime.RuntimeDeps{}, runtime.RuntimeInputs{Tools: reg}, spec, timeout)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
	if rt != nil {
		t.Fatal("expected a nil Runtime on timeout")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("BuildRuntimeWithTimeout took %s, want approximately the %s test timeout, never the real DefaultMCPConnectTimeout", elapsed, timeout)
	}
}

// Construction timeout must close and join an already-connected server before
// returning, and must never publish a partial tool catalogue.
func TestBuildRuntimeWithTimeout_PartialConnectionJoinedBeforeReturn(t *testing.T) {
	binPath := buildMCPServerFixture(t)
	marker := filepath.Join(t.TempDir(), "closed.marker")
	reg := tool.NewRegistry()
	spec := runtime.AgentSpec{
		ID: "a", Name: "A", Model: "anthropic/claude-sonnet-5",
		MCPServers: []mcp.ServerConfig{
			{Name: "healthy", Command: binPath, Args: []string{"-marker=" + marker}},
			{Name: "hung", Command: "sleep", Args: []string{"30"}},
		},
	}
	rt, err := buildRuntimeWithTimeout(t, runtime.RuntimeDeps{}, runtime.RuntimeInputs{Tools: reg}, spec, app.DefaultMCPConnectTimeout)
	if err == nil || rt != nil {
		t.Fatalf("expected timeout: %v %v", rt, err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("connected child not closed before return: %v", err)
	}
	if len(reg.Names()) != 0 {
		t.Fatalf("partial catalogue published: %v", reg.Names())
	}
}
