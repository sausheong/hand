# hand Phase 7 Design: Extensibility — MCP Servers

## Context

hand today has zero extension points: the tool set is fixed at compile
time (`agentio.BuildRegistry`), and there is no plugin, skill, or
template mechanism of any kind. Meanwhile `harness` already ships a
working MCP client (`harness/tools/mcp`), and `runtime.AgentSpec.MCPServers`
is a field `BuildRuntime` already knows how to consume — connecting each
configured server and registering its tools (namespaced
`mcp__<Name>__<tool>`) onto the agent's registry — but
`agentio.BuildAgentSpec` never populates it.

This is deliberately the "simpler than Pi" path to extensibility: Pi
built a bespoke TypeScript extension/skill/prompt-template/package
system to let users add capabilities. hand doesn't need to invent
anything — MCP is an open, already-adopted standard, and the client
support already exists in the library hand is built on. Wiring it up is
almost entirely configuration plumbing, not new mechanism.

## Goals

1. `~/.hand/config.json` gains an optional `mcp_servers` list; each
   entry becomes one `mcp.ServerConfig` passed into
   `AgentSpec.MCPServers`.
2. MCP-provided tools are gated (require approval) by default, since an
   arbitrary server can expose arbitrary mutating tools and hand's
   existing per-tool-name trust model has no basis for trusting a tool
   it's never seen before. A server can be marked `trusted` in config to
   skip gating for everything it provides.
3. A connect failure for a configured server is a clear startup error
   (matching `BuildRuntime`'s existing all-or-nothing behavior for
   `MCPServers` — see `builder.go`, which already tears down every
   connected client and returns an error if any one server fails).
4. A configured server that never responds does not hang hand's startup
   forever.

## Non-goals

- A server registry, marketplace, or auto-discovery. The user names
  exactly the servers they want, in config, same as everything else in
  `config.json`.
- Per-tool (as opposed to per-server) trust. "Trusted" is one switch for
  everything a server exposes; real per-tool-pattern allowlisting is a
  meaningfully bigger feature (same reasoning `permissions`' Phase 2
  spec used to punt on per-command allowlisting).
- OAuth-based MCP authentication. Only stdio servers and static-header
  HTTP servers are supported — exactly what `harness/tools/mcp` itself
  supports today. If harness grows OAuth support later, this is worth
  revisiting.
- Any TUI change to how MCP tools render. They flow through the same
  generic tool-call/result display already built; a per-server preview
  format is a further-out enhancement, not required for extensibility to
  exist at all.

## Design

### `internal/config` changes

```go
// MCPServer is one entry in config.json's mcp_servers list. Trusted is
// hand-side only (not part of mcp.ServerConfig) — it controls approval
// gating, not the connection itself.
type MCPServer struct {
    Name    string            `json:"name"`
    Command string            `json:"command,omitempty"`
    Args    []string          `json:"args,omitempty"`
    Env     map[string]string `json:"env,omitempty"`
    URL     string            `json:"url,omitempty"`
    Headers map[string]string `json:"headers,omitempty"`
    Trusted bool              `json:"trusted,omitempty"`
}

type Config struct {
    // ...existing fields...
    MCPServers []MCPServer `json:"mcp_servers,omitempty"`
}

// ToServerConfigs converts cfg's MCP entries to harness's ServerConfig
// type, dropping the hand-only Trusted flag.
func (cfg Config) ToServerConfigs() []mcp.ServerConfig

// TrustedMCPServers returns the set of server names marked trusted, for
// NewApprovalHook/NewOneShotApprovalHook.
func (cfg Config) TrustedMCPServers() map[string]bool
```

`internal/config` gains a dependency on `harness/tools/mcp` for the
`mcp.ServerConfig` type — the same kind of dependency `internal/agentio`
already has on several `harness/tools/*` packages, so this doesn't
introduce a new layering pattern.

### `internal/agentio` changes

`BuildAgentSpec` gains a parameter:

```go
func BuildAgentSpec(model, workspace string, maxTurns int, fallbackModel string,
    mcpServers []mcp.ServerConfig, hook ...) runtime.AgentSpec
```

Sets `runtime.AgentSpec.MCPServers: mcpServers` (nil/empty is fine — the
existing zero-servers behavior is unchanged).

`approval.go` changes: `NewApprovalHook` and `NewOneShotApprovalHook`
each gain a `trustedServers map[string]bool` parameter (server *names*,
not tool names — see below for why). The gating check becomes:

```go
func isGated(name string, trustedServers map[string]bool) bool {
    if gatedTools[name] {
        return true
    }
    if server, isMCP := mcpServerName(name); isMCP {
        return !trustedServers[server]
    }
    return false // a built-in, non-gated tool
}

// mcpServerName extracts <Name> from harness's "mcp__<Name>__<tool>"
// naming convention (runtime/builder.go). isMCP is true for anything
// carrying the "mcp__" prefix, even a malformed name with no second
// "__" — that returns an empty/unmatchable server string, which can
// never appear in trustedServers, so a malformed MCP tool name fails
// safe as gated rather than silently slipping through ungated.
func mcpServerName(toolName string) (server string, isMCP bool) {
    if !strings.HasPrefix(toolName, "mcp__") {
        return "", false
    }
    rest := strings.TrimPrefix(toolName, "mcp__")
    server, _, _ = strings.Cut(rest, "__")
    return server, true
}
```

This parses the tool name's namespace prefix rather than needing the
connected MCP client's tool list — which isn't available where the hook
is constructed anyway (the hook is built *before* `BuildRuntime` runs
`mcp.Connect`, since it's passed in via `AgentSpec.Loop.Hooks.BeforeToolUse`
for `BuildRuntime` to wire up). Two distinct failure modes worth telling
apart, both documented in a comment on `mcpServerName`:

- A single malformed-but-still-`"mcp__"`-prefixed tool name (covered
  above) — handled: `isMCP` stays `true`, so it's gated.
- Harness changing its namespacing convention *entirely* (dropping the
  `"mcp__"` prefix, or restructuring it) — not something `mcpServerName`
  can detect; every MCP tool would silently stop being recognized as MCP
  and fall through to the ungated built-in-tool path. This is why the
  test suite pins the exact `"mcp__<Name>__<tool>"` format as a literal
  string constant rather than deriving it — a harness upgrade that
  changes the convention fails that hand test loudly, instead of quietly
  un-gating every MCP tool in production.

### `internal/agentio` changes: connect timeout

`mcp.Connect` (called inside `harness.BuildRuntime`, per server, for
every entry in `AgentSpec.MCPServers`) is invoked with
`context.Background()` — confirmed by reading `builder.go` — so a
misbehaving server (an unreachable HTTP endpoint, a stdio command that
never replies to the initial handshake) hangs `hand`'s startup
indefinitely, with no way out short of killing the process. Harness
exposes no per-server timeout knob on `AgentSpec.MCPServers` or
`RuntimeInputs`, and `BuildRuntime` itself takes no `context.Context`
parameter at all — so hand cannot fix this by passing a bounded context
into `BuildRuntime`. The only lever available is wrapping the whole
`BuildRuntime` call:

```go
// DefaultMCPConnectTimeout bounds how long hand waits for BuildRuntime
// to finish connecting every configured MCP server before giving up.
// BuildRuntime takes no context, so this wraps the call itself rather
// than threading a deadline through it. Exported (not a bare const) so
// tests can pass a short timeout instead of waiting out the real one.
const DefaultMCPConnectTimeout = 15 * time.Second

func buildRuntimeWithTimeout(deps runtime.RuntimeDeps, inputs runtime.RuntimeInputs, spec runtime.AgentSpec, timeout time.Duration) (*runtime.Runtime, error) {
    if len(spec.MCPServers) == 0 {
        return runtime.BuildRuntime(deps, inputs, spec) // no servers, no risk, no timeout overhead
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
        return nil, fmt.Errorf("timed out after %s connecting configured MCP servers (check mcp_servers in ~/.hand/config.json)", timeout)
    }
}
```

This is a real limitation, not a clean fix: the abandoned
`runtime.BuildRuntime` goroutine keeps running in the background (Go has
no way to forcibly cancel a goroutine that isn't itself watching a
context), so a hung stdio child process spawned by `mcp.Connect` outlives
the timeout and is never cleaned up. Accepted for this phase — the
alternative (hand reimplementing its own MCP connection logic just to
get cancellation) is a much bigger feature than "wire up the client
harness already has." Flagged here explicitly so it isn't rediscovered
as a surprise later: **a hung MCP server can still leak a subprocess for
the life of the hand process**, even though the CLI itself becomes
responsive again after the timeout.

`cmd/hand/main.go` calls `agentio.buildRuntimeWithTimeout(deps, inputs, spec, agentio.DefaultMCPConnectTimeout)`
in place of calling `runtime.BuildRuntime` directly (or this could live
as a wrapper inside `agentio` and be exported — naming/placement is an
implementation
detail, not a design decision this spec needs to pin down further).

### `cmd/hand/main.go` changes

```go
mcpServers := cfg.ToServerConfigs()
trustedServers := cfg.TrustedMCPServers()
...
if oneShot {
    hook = agentio.NewOneShotApprovalHook(perms, *yesFlag, trustedServers)
} else {
    sender = &programSender{}
    hook = agentio.NewApprovalHook(sender, perms, workspace, trustedServers)
}
spec := agentio.BuildAgentSpec(model, workspace, maxTurns, fallbackModel, mcpServers, hook)
```

`RuntimeInputs.Tools` must be the concrete `*tool.Registry` type for
`BuildRuntime` to register MCP tools onto it (it type-asserts internally
and silently skips registration otherwise, per harness's own comment in
`builder.go`) — `agentio.BuildRegistry` already returns `*tool.Registry`,
so this is already satisfied; called out here only so a future refactor
away from `*tool.Registry` doesn't silently break MCP tool registration.

A connect failure during `runtime.BuildRuntime(...)` is already handled
by the existing `return fmt.Errorf("build runtime: %w", err)` — no new
error-handling path needed, just confirming the existing one now also
covers "misconfigured MCP server."

## Testing

- `internal/config`: `Config.MCPServers` JSON round-trip (including
  `trusted`); `ToServerConfigs` field-by-field mapping;
  `TrustedMCPServers` returns only the servers with `Trusted: true`.
- `internal/agentio`:
  - `mcpServerName` unit tests: `"mcp__github__create_issue"` →
    `("github", true)`; `"bash"` → `("", false)`; a malformed
    `"mcp__no_double_underscore"` → `("", true)` — still reported as MCP
    (so `isGated` gates it) even though no server name could be parsed
    out of it.
  - `NewApprovalHook`/`NewOneShotApprovalHook`: an `mcp__untrusted__tool`
    call behaves like `bash` (prompts / denied-by-default); an
    `mcp__trusted__tool` call (its server name present in
    `trustedServers`) is allowed with no prompt, same as an ungated
    built-in tool; existing gated/ungated built-in-tool test cases are
    unaffected (regression check on the added parameter).
  - `buildRuntimeWithTimeout`: zero `MCPServers` calls straight through
    with no timeout goroutine involved (assert via a fast fake that would
    fail the test if it took the timeout path); a normal, quickly-resolving
    call still returns successfully when given a short test timeout (e.g.
    `100 * time.Millisecond`) well above how long the fake setup actually
    takes; a deliberately slow path (a stdio "server" that's just
    `sleep 30`, or any fixture that simply never returns) given a short
    test timeout (e.g. `50 * time.Millisecond`) returns the timeout error
    at approximately that duration — fast, because `timeout` is a real
    parameter the test controls, never `DefaultMCPConnectTimeout`'s real
    15 seconds.
- No new `internal/tui` tests — MCP tools reuse the existing generic
  tool-call/result rendering path, already covered.
