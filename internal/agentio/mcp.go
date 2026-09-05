package agentio

import (
	"fmt"
	"time"

	"github.com/sausheong/harness/runtime"
)

// DefaultMCPConnectTimeout bounds how long hand waits for BuildRuntime
// to finish connecting every configured MCP server before giving up.
// BuildRuntime takes no context.Context, and harness exposes no
// per-server connect timeout to thread through instead, so this wraps
// the whole call rather than a deadline inside it. Exported (not a bare
// const) so tests can pass a short timeout instead of waiting out the
// real one.
const DefaultMCPConnectTimeout = 15 * time.Second

// BuildRuntimeWithTimeout wraps runtime.BuildRuntime with a deadline on
// how long it may take to connect every server in spec.MCPServers. A
// spec with no MCP servers calls straight through with no goroutine or
// timeout overhead — there's nothing that can hang.
//
// This is a real limitation, not a clean fix: on timeout, the abandoned
// BuildRuntime goroutine keeps running in the background (Go has no way
// to forcibly cancel a goroutine that isn't itself watching a context),
// so a genuinely hung stdio child process spawned by mcp.Connect
// outlives the timeout and is never cleaned up — that specific case can
// leak a subprocess for the life of the hand process, even though the
// CLI itself becomes responsive again after the timeout. Accepted for
// this phase: reimplementing MCP connection logic just to get
// cancellation is a much bigger feature than wiring up the client
// harness already has.
//
// What this function DOES clean up: a server that's merely slow, not
// permanently hung, and connects successfully a few seconds after the
// timeout already fired and returned an error to the caller. Without
// the drain goroutine below, that late-arriving *runtime.Runtime — with
// its live MCP client connections — would simply be discarded with no
// Close() ever called, leaking those connections (and any stdio
// subprocess) for good even though the connection itself was never
// actually hung.
func BuildRuntimeWithTimeout(deps runtime.RuntimeDeps, inputs runtime.RuntimeInputs, spec runtime.AgentSpec, timeout time.Duration) (*runtime.Runtime, error) {
	if len(spec.MCPServers) == 0 {
		return runtime.BuildRuntime(deps, inputs, spec)
	}

	type result struct {
		rt  *runtime.Runtime
		err error
	}
	done := make(chan result, 1)
	go func() {
		rt, err := runtime.BuildRuntime(deps, inputs, spec)
		done <- result{rt, err}
	}()

	select {
	case r := <-done:
		return r.rt, r.err
	case <-time.After(timeout):
		go func() {
			if r := <-done; r.rt != nil {
				_ = r.rt.Close()
			}
		}()
		return nil, fmt.Errorf("timed out after %s connecting configured MCP servers (check mcp_servers in ~/.hand/config.json)", timeout)
	}
}
