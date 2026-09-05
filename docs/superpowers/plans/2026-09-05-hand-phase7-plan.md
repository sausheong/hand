# hand Phase 7 (Extensibility — MCP Servers) Implementation Plan

**Goal:** Wire `~/.hand/config.json`'s `mcp_servers` list into
`runtime.AgentSpec.MCPServers`, gate MCP-provided tools by default
(per-server trust opt-out), and bound MCP connect time at startup.

**Spec:** [docs/superpowers/specs/2026-09-05-hand-phase7-design.md](../specs/2026-09-05-hand-phase7-design.md)

Note: this phase is "wire up the client harness already has," not new
mechanism — no server registry, no per-tool trust, no OAuth-based MCP
auth, and no TUI rendering changes for MCP tool calls (they reuse the
existing generic tool-call/result display). See the spec's Non-goals for
the full rationale; do not expand scope to cover any of these.

## Tasks

1. **`internal/config` package** — `MCPServer` struct (`Name`, `Command`,
   `Args`, `Env`, `URL`, `Headers`, `Trusted`); `Config.MCPServers
   []MCPServer` field; `Config.ToServerConfigs() []mcp.ServerConfig`
   (drops the hand-only `Trusted` flag); `Config.TrustedMCPServers()
   map[string]bool`. Takes on a new dependency on `harness/tools/mcp` for
   `mcp.ServerConfig` — consistent with `internal/agentio`'s existing
   dependencies on `harness/tools/*`, not a new layering pattern. Unit
   tests per the spec's Testing section (JSON round-trip including
   `trusted`, `ToServerConfigs` field-by-field mapping, `TrustedMCPServers`
   returning only `Trusted: true` entries).
2. **`internal/agentio` — `BuildAgentSpec` + approval gating** —
   `BuildAgentSpec` gains an `mcpServers []mcp.ServerConfig` parameter,
   set on `runtime.AgentSpec.MCPServers` (nil/empty preserves today's
   zero-servers behavior unchanged). `approval.go`: `NewApprovalHook` and
   `NewOneShotApprovalHook` each gain a `trustedServers map[string]bool`
   parameter; add `mcpServerName(toolName string) (server string, isMCP
   bool)` parsing harness's `"mcp__<Name>__<tool>"` prefix, and `isGated`
   consults it (an MCP tool is gated unless its server is in
   `trustedServers`). Note the spec's explicit distinction between two
   failure modes: a single malformed-but-prefixed tool name is handled
   (`isMCP` stays true, so it's gated — fails safe); harness dropping or
   restructuring the `"mcp__"` convention entirely is *not* detectable by
   `mcpServerName` and would silently un-gate every MCP tool — this is why
   the test suite must pin the exact `"mcp__<Name>__<tool>"` format as a
   literal string constant rather than deriving it, so a harness upgrade
   that changes the convention fails the hand test loudly. Update
   `approval_test.go`'s existing call sites for the new parameter (regression
   check that ungated/gated built-in-tool cases are unaffected); add cases
   per the spec's Testing section for `mcpServerName` and for trusted vs.
   untrusted MCP tool calls on both hooks.
3. **`internal/agentio` — MCP connect timeout** — `DefaultMCPConnectTimeout
   = 15 * time.Second` (exported, not a bare const, so tests can pass a
   short timeout) and `buildRuntimeWithTimeout(deps, inputs, spec,
   timeout)`, wrapping `runtime.BuildRuntime` in a goroutine + `select`
   since `BuildRuntime` takes no `context.Context` and harness exposes no
   per-server connect timeout to thread through instead; the
   zero-`MCPServers` case calls `runtime.BuildRuntime` directly with no
   goroutine/timeout overhead. Flag in a doc comment the accepted
   limitation the spec calls out explicitly: on timeout, the abandoned
   `BuildRuntime` goroutine keeps running — a hung MCP server (e.g. a
   stdio child process stuck in its handshake) can leak a subprocess for
   the life of the hand process even though the CLI itself becomes
   responsive again. This is accepted for this phase, not silently
   dropped — do not attempt to fix it by having hand reimplement MCP
   connection logic itself. Unit tests per the spec's Testing section:
   zero servers skips the timeout path entirely (assert via a fake that
   would fail the test if it took the timeout branch); a normal
   fast-resolving call succeeds under a short test timeout; a
   deliberately-hanging fixture (e.g. a stdio "server" that's just `sleep
   30`) returns the timeout error at approximately the short test
   timeout, never waiting out the real `DefaultMCPConnectTimeout`.
4. **`cmd/hand/main.go` wiring** — `cfg.ToServerConfigs()` and
   `cfg.TrustedMCPServers()` computed once; pass `trustedServers` into
   both `NewApprovalHook` and `NewOneShotApprovalHook` call sites; pass
   `mcpServers` into `BuildAgentSpec`; call
   `agentio.buildRuntimeWithTimeout(deps, inputs, spec,
   agentio.DefaultMCPConnectTimeout)` in place of the direct
   `runtime.BuildRuntime` call (naming/placement of this wrapper —
   unexported helper vs. exported — is an implementation detail the spec
   deliberately leaves open; pick one and move on). No new error-handling
   path needed: a connect failure during the wrapped `BuildRuntime` call
   already surfaces through the existing `return fmt.Errorf("build
   runtime: %w", err)`. Confirm (comment only, no code change expected)
   that `RuntimeInputs.Tools` stays the concrete `*tool.Registry` type
   `agentio.BuildRegistry` already returns — `BuildRuntime` type-asserts
   this internally to register MCP tools and silently skips registration
   otherwise, per harness's own comment in `builder.go`.
5. **Verify** — `go build ./...`, `go vet ./...`, `go test ./...` across
   the whole module.
