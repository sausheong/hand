# Joined MCP startup timeout

The staged Hand wrapper now uses Harness BuildRuntimeContext with a deadline. It no longer abandons a construction goroutine on timeout. Harness closes and joins partial MCP connections before returning failure and publishes discovered tools only after construction succeeds. The wrapper preserves the no-server zero-timeout fast path and wraps deadline errors without losing their identity.

The permanent MCP regression now connects a real healthy stdio fixture followed by a hung server and requires the healthy child's closure marker to exist before the call returns. The fixture exposes an actual echo tool: successful construction must register it, while failed construction must leave the registry empty. This replaces the obsolete test expectation that a timed-out constructor eventually succeeds and is closed later.

Agentio race-suite and vet results, raw output and source hashes are recorded in `mcp-joined-integration.json`. Optional-server degradation and displaying the UI before MCP construction remain unfinished. Full native/released-candidate qualification remains pending. The aggregate patch excludes the temporary go.mod replacement and requires development Harness commit `540dd08a6975ac0076f697668ca861468715cf44`.
