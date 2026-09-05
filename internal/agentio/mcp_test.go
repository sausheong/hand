package agentio_test

import (
	"fmt"
	"os"
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
//
// Deliberately os.MkdirTemp, not t.TempDir(): the sync.Once below runs
// exactly once for the whole test binary, on whichever test happens to
// call this function first — a t.TempDir() would tie the binary's
// lifetime to that *specific* test and get deleted by its cleanup the
// moment that one test finishes, breaking every other test that reuses
// the cached path afterward. Left for the OS's own temp-dir GC, same as
// any other build-once-per-process test fixture.
func buildMCPServerFixture(t *testing.T) string {
	t.Helper()
	mcpServerFixtureOnce.Do(func() {
		dir, err := os.MkdirTemp("", "hand-mcpserver-fixture")
		if err != nil {
			mcpServerFixtureErr = fmt.Errorf("create fixture temp dir: %w", err)
			return
		}
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

// Regression: on timeout, the abandoned BuildRuntime goroutine's
// eventual result used to be discarded outright — a server that's
// merely slow (not permanently hung) and connects successfully a moment
// after the timeout fired leaked its live MCP connection (and spawned
// subprocess) forever, since nothing ever called Close() on the
// late-arriving *runtime.Runtime. This drives a real, slightly-delayed
// stdio server through the timeout path and checks — via the fixture
// writing a marker file only once its Run() returns, which happens when
// the client side closes the connection — that hand's drain goroutine
// actually closes that late connection instead of abandoning it.
func TestBuildRuntimeWithTimeout_LateSuccessAfterTimeoutIsStillClosed(t *testing.T) {
	binPath := buildMCPServerFixture(t)
	marker := filepath.Join(t.TempDir(), "closed.marker")
	reg := tool.NewRegistry()
	spec := runtime.AgentSpec{
		ID: "a", Name: "A", Model: "anthropic/claude-sonnet-5",
		MCPServers: []mcp.ServerConfig{{
			Name:    "slow",
			Command: binPath,
			Args:    []string{"-delay=300ms", "-marker=" + marker},
		}},
	}

	const timeout = 50 * time.Millisecond
	rt, err := agentio.BuildRuntimeWithTimeout(runtime.RuntimeDeps{}, runtime.RuntimeInputs{Tools: reg}, spec, timeout)
	if err == nil {
		t.Fatal("expected a timeout error since the fixture's 300ms delay exceeds the 50ms timeout")
	}
	if rt != nil {
		t.Fatal("expected a nil Runtime on timeout")
	}

	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			return // the fixture observed its connection being closed
		}
		select {
		case <-deadline:
			t.Fatal("marker file never appeared: the late-succeeding connection was never closed")
		case <-time.After(20 * time.Millisecond):
		}
	}
}
