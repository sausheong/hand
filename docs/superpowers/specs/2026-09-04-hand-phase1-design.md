# hand Phase 1 Design: Core Loop + TUI Shell

## Context

`hand` is a new CLI coding agent built on [`harness`](../../../../harness),
Anthropic's Go agentic-loop library. The goal is an interactive terminal
experience similar to Claude Code: a persistent REPL where the model can
read/write files, run shell commands, fetch/search the web, and track its
own todos, streaming its output live into a terminal UI.

The full product is larger than one spec can usefully cover, so it is being
built in four phases:

1. **Core loop + TUI shell** (this document) — config, tool registry,
   Bubble Tea shell, streaming output, a bare-minimum y/n approval gate.
2. **Permissions** — allow/deny/always-allow prompts with a persisted
   project-local allowlist.
3. **Sessions** — JSONL persistence via `session.Store`, auto-resume last
   session per directory, session listing/switching.
4. **One-shot mode + polish** — `-p` flag for non-interactive runs, diff
   rendering for edits, spinners, multi-provider switch UX.

Each phase gets its own brainstorm → spec → plan → implementation cycle.
This document covers Phase 1 only.

## Goals

- `hand` run from any directory gives an interactive chat loop against
  any of harness's four providers (Anthropic, OpenAI, Gemini, Qwen),
  operating on that directory as its workspace.
- The agent can read/write/edit files, run bash commands, fetch/search the
  web, and maintain a todo list — the four tool packages selected for v1.
- Output streams live into a real terminal UI (Bubble Tea), not raw stdout.
- Before any file write, edit, or bash execution, the user is asked to
  approve it (no persistence of the answer yet — that is Phase 2).
- Config (default provider/model) lives in `~/.hand/config.json` so
  Phase 2 can extend the same file family for the permission allowlist.

## Non-goals (deferred to later phases or out of scope entirely)

- Persisted "always allow" decisions (Phase 2).
- Session save/resume (Phase 3).
- One-shot / non-interactive mode (Phase 4).
- Slash commands, plan mode, subagents, memory/skills, MCP servers,
  browser tool — none of these are in scope for `hand` v1 as currently
  planned; revisit only if a future phase is explicitly proposed.

## Architecture

`hand` is a single Go binary (module `github.com/sausheong/hand`,
entrypoint `cmd/hand`) that composes one harness `Runtime` per process
and drives it from a Bubble Tea terminal UI
(`charmbracelet/bubbletea` + `charmbracelet/bubbles` +
`charmbracelet/lipgloss`). The workspace is always the current working
directory — there is no `--workspace` flag in Phase 1.

Harness owns the agent loop, streaming, context compaction, and tool
dispatch. `hand` owns configuration, the tool registry wiring, the
approval bridge, and the TUI. Nothing in `internal/tui` talks to an LLM
provider directly; everything goes through the `Runtime`.

### The approval bridge (the one non-obvious piece)

`Runtime.Run(ctx, msg, images)` spawns its own goroutine internally and
returns a `<-chan runtime.AgentEvent` immediately (see
`harness/runtime/runtime.go`). `LifecycleHooks.BeforeToolUse` fires
synchronously *on that internal goroutine*, not on the goroutine that
called `Run`. That goroutine is not the Bubble Tea event loop, so an
approval prompt cannot simply return a value from `Update()`.

The bridge:

```go
type ApprovalRequest struct {
    Tool    string
    Input   json.RawMessage
    Respond chan bool // hook blocks reading this
}
```

`BeforeToolUse` is implemented as a closure that, for `write_file`,
`edit_file`, and `bash` only, builds an `ApprovalRequest`, calls
`program.Send(req)` (Bubble Tea's `tea.Program.Send` is documented safe
to call from any goroutine — this is its intended use for external
events), and then blocks on `<-req.Respond`. All other tools
(`read_file`, `web_fetch`, `web_search`, `todo_write`) return
`HookDecision{Allow: true}` immediately with no prompt.

The Bubble Tea `Model.Update()` receives the `ApprovalRequest` as a
`tea.Msg`, stores it as pending state, and renders an inline prompt in
the transcript (`Allow bash: "go test ./..." ? [y/N]`). On the next `y`
or `n` keypress (while a request is pending, the textarea is not
accepting normal input), `Update()` writes `true`/`false` into
`req.Respond` and clears the pending state. This unblocks the hook,
which returns `HookDecision{Allow: <answer>}` to the runtime.

No decision is cached. The identical prompt fires again next time, even
for the same command in the same turn — that is explicitly correct for
Phase 1 and is what Phase 2's allowlist replaces.

### Event flow

1. User types into the `textarea`, presses Enter. A `tea.Cmd` calls
   `rt.Run(ctx, text, nil)`, receives the event channel, and starts a
   reader goroutine that ranges over it, calling `program.Send(ev)` for
   each `runtime.AgentEvent`.
2. `EventTextDelta` appends to the transcript buffer backing the
   `viewport`; the model re-renders on each message (no manual
   char-by-char terminal writes).
3. `EventToolCallStart` appends a `[tool: name]` line. If the tool is one
   of the three gated tools, the approval bridge described above runs
   before harness ever calls the tool's `Execute`, so the prompt appears
   before any side effect happens.
4. `EventToolResult` appends a ✓ (or ✗ with the error text) line.
5. `EventError` appends an error line and returns control to the input
   box; it does not crash the program.
6. `EventDone` stops the spinner, re-enables the textarea, and (if
   `ev.Usage` is set) can optionally show a token count in the status
   line.
7. Ctrl-C while a turn is in flight cancels the run's `context.Context`;
   the resulting error is rendered like any other `EventError`.

## Components

- **`cmd/hand/main.go`** — flag parsing (`--model provider/model`
  overrides config), loads `internal/config`, constructs the selected
  provider, builds the tool registry via `internal/agentio`, calls
  `runtime.BuildRuntime`, launches the Bubble Tea program.
- **`internal/config`** — `Load()` / `Save()` for
  `~/.hand/config.json`. Schema: `{"model": "anthropic/claude-sonnet-5"}`
  (provider is encoded in the `provider/model` string per
  `llm.ParseProviderModel`). `Load()` creates the file with a default
  model on first run if it does not exist. A `--model` flag value
  overrides the loaded config for that invocation only (does not rewrite
  the file). API keys are never stored in this file — they come from
  each provider's standard environment variable
  (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, Google ADC for Gemini,
  Qwen's DashScope key), matching harness's own convention.
- **`internal/agentio`** — builds the `tool.Registry` (all four
  `WorkDir`-scoped to the cwd: `file.ReadFileTool`, `file.WriteFileTool`,
  `file.EditFileTool`, `bash.BashTool` with `ExecPolicy` nil (full — the
  approval gate is the safety net here, not the exec policy),
  `web.WebFetchTool`, `web.WebSearchTool`, `todo.TodoWriteTool`);
  constructs the approval bridge and its `BeforeToolUse` closure;
  assembles the single `runtime.AgentSpec` used for the process
  (`MaxTurns`, system prompt tuned for coding, `Workspace` = cwd).
- **`internal/tui`** — the Bubble Tea `Model`: `viewport` (scrollback
  transcript), `textarea` (multi-line input), a status line (provider,
  model, spinner while a turn is running), and pending-approval state.
- **`internal/tui/events.go`** — the `runtime.AgentEvent` → `tea.Msg`
  reader goroutine described above, plus the `ApprovalRequest` message
  type and its rendering/keypress handling.

## Error handling

- Startup errors (missing/invalid API key for the selected provider,
  malformed `--model` string, config file that fails to parse) print a
  plain message to stderr and exit non-zero — the TUI never launches in
  a broken state.
- Mid-run errors (`EventError`, individual tool errors, a denied
  approval, Ctrl-C cancellation) render as a line in the transcript and
  return the program to an interactive, input-accepting state. Nothing
  in Phase 1 crashes the process on an agent-loop error.
- A denied approval reaches the model as a normal tool-result error
  (the `Reason` from `HookDecision`), exactly like harness's existing
  `PermissionChecker`-denial shape — the model sees it and can adapt
  (ask again with a different command, explain why it needed it, etc.)
  rather than the run silently stalling.

## Testing

- `internal/config`: unit tests for default-on-first-run, load/save
  round-trip, and `--model` flag overriding the loaded value without
  persisting it.
- `internal/agentio`: unit test the approval-bridge hook in isolation —
  a fake message sink standing in for `tea.Program.Send`, asserting the
  hook blocks until `Respond` fires and that the returned
  `HookDecision.Allow` matches the response; assert `read_file` /
  `web_fetch` / `web_search` / `todo_write` never trigger a request.
- `internal/tui`: Bubble Tea models are testable with Charm's `teatest`
  helper — cases for "approval prompt appears on a `write_file` tool
  call and blocks the textarea", "streamed text deltas accumulate in the
  transcript", "Ctrl-C during a run returns to the input state".
- No live-API integration tests in this phase (matches harness's own
  `//go:build live` convention, which is opt-in and not run by default).
  Manual verification against a real API key is the acceptance check for
  the end-to-end experience, same as how harness's own example agents
  are verified.

## Open questions carried into Phase 2

- Exact shape of the persisted allowlist file (likely
  `.hand/settings.json`, project-local) and how it composes with the
  user-level `~/.hand/config.json` — not decided here, deliberately
  deferred.
- Whether "always allow" should be scoped per exact command (for bash)
  or per tool name — a bash allowlist granular enough to be safe but not
  so granular it prompts on every trivial variation needs its own
  design pass.
