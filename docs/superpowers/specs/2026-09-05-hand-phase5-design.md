# hand Phase 5 Design: Reliability — Compaction, Fallback Model, and Usage Visibility

## Context

Phases 1-4 shipped the interactive loop, permissions, sessions, and one-shot
mode. Three related reliability gaps remain, discovered by reading how
`agentio.BuildAgentSpec` and `cmd/hand/main.go` wire up `runtime.BuildRuntime`:

1. `runtime.RuntimeInputs.Compaction` is never set, so `Runtime.Compaction`
   is `nil`. Two consequences: `/compact` (added in the TUI-polish work)
   always reports `skipped: no_summarizer`, and — more seriously —
   harness's own `recoverFromPreTokenError` only attempts automatic
   recovery from a context-overflow error `if r.Compaction != nil`. With
   it nil, a session that fills its context window just hard-fails with a
   raw provider error and no recovery path.
2. `runtime.AgentSpec.FallbackModel` is never set, so a transient
   provider error (429/5xx) fails the whole turn instead of retrying
   against a fallback model, a mechanism harness already implements.
3. `runtime.AgentEvent`'s terminal `EventDone` carries `Usage *llm.Usage`
   (token counts) and `internal/tui`'s `handleAgentEvent` discards it —
   there is no way to see how much of a session's context or budget has
   been used.

## Goals

1. Wire a `compaction.Manager` (with a `compaction.Summarizer`) into
   `RuntimeInputs.Compaction` so `/compact` actually compacts, and
   automatic reactive/preventive compaction can kick in.
2. Make `FallbackModel` configurable (flag + config), same pattern as
   `model`/`base_url`/`max_turns`.
3. Capture `EventDone.Usage` in the TUI and expose it via a `/usage`
   command.

## Non-goals

- Cross-provider fallback — harness's `BuildRuntime` already validates
  `FallbackModel` is same-provider as `Model` and warns+ignores it
  otherwise; hand does not duplicate that check.
- A dollar-cost calculator. Provider pricing changes too often for a
  minimal tool to hardcode; this phase shows token counts only.
- A dedicated compaction model/config knob. The summarizer reuses the
  *same* provider and model as the active run — no new API key or config
  surface. A cheaper dedicated summarizer model is a reasonable future
  addition but adds a config dimension this phase skips.
- A persistent, always-visible token counter in the status line. `/usage`
  is opt-in, matching hand's muted, ask-don't-tell aesthetic elsewhere.
- Configurable compaction thresholds (`PreserveTurns`, `Threshold`,
  `MessageCap`) beyond harness's built-in defaults. Can be added later as
  `config.json` fields if the defaults prove wrong in practice.

## Design

### `internal/config` changes

```go
type Config struct {
    Model         string `json:"model"`
    BaseURL       string `json:"base_url,omitempty"`
    MaxTurns      int    `json:"max_turns,omitempty"`
    FallbackModel string `json:"fallback_model,omitempty"` // "provider/model" form, same as Model
}
```

`FallbackModel` is stored in the same `"provider/model"` form as `Model`
(not a bare model name) — `runtime.AgentSpec.FallbackModel` expects that
form and does its own same-provider validation internally, so hand just
passes it through unparsed.

```go
// ResolveFallbackModel returns flagValue if non-empty, otherwise
// cfg.FallbackModel. Empty means "no fallback" — the current behavior.
func ResolveFallbackModel(flagValue string, cfg Config) string
```

Same shape as `ResolveModel`/`ResolveBaseURL`.

### `internal/agentio` changes

`BuildAgentSpec` gains a parameter:

```go
func BuildAgentSpec(model, workspace string, maxTurns int, fallbackModel string, hook ...) runtime.AgentSpec
```

Sets `runtime.AgentSpec.FallbackModel: fallbackModel` (empty is fine —
harness treats `""` as "no fallback").

New file `compaction.go`:

```go
// BuildCompactionManager returns a compaction.Manager whose Summarizer
// reuses the same provider and model as the active run — the simplest
// thing that works, with no new API key or config surface. model is the
// bare model name (no provider prefix), matching Summarizer.Model.
func BuildCompactionManager(provider llm.LLMProvider, model string) *compaction.Manager {
    return &compaction.Manager{
        Summarizer: &compaction.Summarizer{Provider: provider, Model: model},
    }
}
```

`PreserveTurns`/`Threshold`/`MessageCap` are left zero — harness's
`Manager.MaybeCompact` and the runtime's preventive-compaction check both
document sane defaults for the zero value (`K = 4` turns preserved; the
runtime's own 0.6 threshold default applies — confirmed in
`runtime.go`'s `maybeKickoffAsyncCompaction`).

**Open question to resolve before implementing, not just assumed:**
reusing the same `llm.LLMProvider` instance means a background
`MaybeCompactAsync` goroutine can call `ChatStream` on it while the main
turn's own `ChatStream` call is still in flight on that same instance —
harness's `Manager.MaybeCompactAsync` explicitly runs detached with its
own context specifically to support this overlap. Nothing in
`llm.LLMProvider` or the `providers/*` packages documents whether a
provider implementation is safe for concurrent calls. It's likely fine
— they read as thin, stateless HTTP-client wrappers, and `http.Client`
itself is safe for concurrent use — but this phase should confirm that
by reading the provider implementation(s) before shipping, not assume
it. If a provider turns out not to be safe, the fallback is a
provider-side mutex in `BuildCompactionManager` (accepting serialized,
slightly-delayed compaction) rather than skipping compaction reuse
entirely.

### `cmd/hand/main.go` changes

- New flag: `--fallback-model` (string, same help-text pattern as
  `--model`).
- `fallbackModel := config.ResolveFallbackModel(*fallbackModelFlag, cfg)`,
  passed into `agentio.BuildAgentSpec`.
- `_, bareModel := llm.ParseProviderModel(model)` (the provider half is
  already extracted a few lines above as `providerName`), then:
  ```go
  compactionMgr := agentio.BuildCompactionManager(provider, bareModel)
  ```
  passed as `runtime.RuntimeInputs.Compaction: compactionMgr`.

No `internal/tui` change is needed for `/compact` itself — `tui.Controller.Compact`
already calls `c.Rt.Compaction.MaybeCompact(...)`; it simply starts
succeeding once `Rt.Compaction` is non-nil instead of always hitting the
`no_summarizer` skip path.

### `internal/tui` changes (usage visibility)

`Model` gains a field:

```go
lastUsage *llm.Usage // nil until the first turn completes with usage reported
```

`handleAgentEvent`'s `EventDone` case captures it: `m.lastUsage = ev.Usage`
(the provider may not report usage at all, in which case `ev.Usage` stays
nil and `/usage` says so).

New slash command `/usage` (added to `commandDefs` in `commands.go`,
which also updates `/help` and the auto-complete dropdown for free):

```go
func (m *Model) runUsageCommand() {
    if m.lastUsage == nil {
        m.transcript = append(m.transcript, toolCallStyle.Render("no usage recorded yet"))
        return
    }
    u := m.lastUsage
    line := fmt.Sprintf("input: %d  output: %d  cache write: %d  cache read: %d",
        u.InputTokens, u.OutputTokens, u.CacheCreationInputTokens, u.CacheReadInputTokens)
    m.transcript = append(m.transcript, toolCallStyle.Render(line))
}
```

## Testing

- `internal/config`: `ResolveFallbackModel` table test (flag empty →
  config value; flag set → overrides; both empty → `""`).
- `internal/agentio`: `BuildAgentSpec` test asserts `spec.FallbackModel`
  matches the passed value; `BuildCompactionManager` test asserts the
  returned `*compaction.Manager` is non-nil with `Summarizer.Provider`
  and `Summarizer.Model` set to what was passed in. Before writing any of
  this, read the concrete `providers/*` implementation(s) hand actually
  ships to confirm concurrent-call safety (see the open question above)
  — this is a read, not a test, but it gates whether `BuildCompactionManager`
  needs a serializing wrapper.
- `internal/tui`: `EventDone` with a populated `Usage` sets
  `m.lastUsage`; `/usage` with no prior usage renders "no usage recorded
  yet"; `/usage` after a `Usage`-bearing `EventDone` renders the token
  counts. `Controller`'s existing
  `TestController_Compact_NoManagerSkipsCleanly` is unchanged (it
  specifically covers the nil-`Compaction` case, which is still reachable
  whenever `main.go`'s summarizer isn't configured — e.g. hypothetically
  in a future no-provider mode) and stays green.
