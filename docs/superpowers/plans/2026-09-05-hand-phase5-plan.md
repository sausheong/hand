# hand Phase 5 (Reliability) Implementation Plan

**Goal:** Wire a `compaction.Manager` and configurable `FallbackModel` into
the runtime, and surface `EventDone.Usage` via a `/usage` command.

**Spec:** [docs/superpowers/specs/2026-09-05-hand-phase5-design.md](../specs/2026-09-05-hand-phase5-design.md)

## Tasks

1. **`internal/config` package** — add `FallbackModel` field to `Config`
   (`json:"fallback_model,omitempty"`, same `"provider/model"` form as
   `Model`, passed through unparsed — hand does not parse it, harness
   does its own same-provider validation); add
   `ResolveFallbackModel(flagValue string, cfg Config) string` following
   the existing `ResolveModel`/`ResolveBaseURL` pattern. Unit tests per
   the spec's Testing section (table test: flag empty → config value;
   flag set → overrides; both empty → `""`).
2. **`internal/agentio.BuildAgentSpec`** — add a `fallbackModel string`
   parameter; set `runtime.AgentSpec.FallbackModel: fallbackModel` (empty
   is fine — harness treats `""` as "no fallback"). Non-goal from the
   spec: hand does **not** duplicate harness's `BuildRuntime`
   same-provider check for `FallbackModel` — that validation already
   lives in harness. Update call sites; add a test asserting
   `spec.FallbackModel` matches the passed value.
3. **`internal/agentio/compaction.go` (new file)** — `BuildCompactionManager(provider
   llm.LLMProvider, model string) *compaction.Manager`, returning a
   `compaction.Manager` whose `Summarizer` reuses the *same* provider and
   model as the active run (bare model name, no provider prefix, matching
   `Summarizer.Model`); leave `PreserveTurns`/`Threshold`/`MessageCap`
   zero to take harness's built-in defaults. Non-goal from the spec: no
   dedicated compaction model/config knob — no new API key or config
   surface this phase. **Open question the spec flags as needing
   resolution before implementing, not assumed:** confirm whether hand's
   concrete `providers/*` implementations are safe for concurrent calls —
   a background `MaybeCompactAsync` goroutine can call `ChatStream` on
   the same provider instance while the main turn's own `ChatStream` call
   is still in flight on it. Read the provider implementation(s) hand
   ships before writing this task's tests. If a provider turns out not to
   be safe, add a serializing wrapper (provider-side mutex) inside
   `BuildCompactionManager` rather than skipping compaction reuse
   entirely. Unit tests per the spec's Testing section: asserts the
   returned `*compaction.Manager` is non-nil with `Summarizer.Provider`
   and `Summarizer.Model` set to what was passed in.
4. **`cmd/hand/main.go` wiring** — new `--fallback-model` flag (same
   help-text pattern as `--model`); `fallbackModel :=
   config.ResolveFallbackModel(*fallbackModelFlag, cfg)` passed into
   `agentio.BuildAgentSpec`; extract the bare model name via
   `llm.ParseProviderModel` (the provider half is already extracted a few
   lines above as `providerName`) and construct `compactionMgr :=
   agentio.BuildCompactionManager(provider, bareModel)`, passed as
   `runtime.RuntimeInputs.Compaction`. No `internal/tui` change is needed
   for `/compact` itself — `tui.Controller.Compact` already calls
   `c.Rt.Compaction.MaybeCompact(...)`; it simply starts succeeding once
   `Rt.Compaction` is non-nil instead of always hitting the
   `no_summarizer` skip path.
5. **`internal/tui` usage visibility** — `Model` gains `lastUsage
   *llm.Usage` (nil until the first turn completes with usage reported);
   `handleAgentEvent`'s `EventDone` case captures `ev.Usage`; new
   `/usage` slash command added to `commandDefs` in `commands.go`
   (updates `/help` and the auto-complete dropdown for free), rendering
   `input:`/`output:`/`cache write:`/`cache read:` token counts, or "no
   usage recorded yet" if `lastUsage` is nil. Non-goals from the spec: no
   dollar-cost calculator (provider pricing changes too often for a
   minimal tool to hardcode — token counts only) and no persistent,
   always-visible status-line counter — `/usage` stays opt-in, matching
   hand's muted, ask-don't-tell aesthetic elsewhere. Update
   `model_test.go`: `EventDone` with a populated `Usage` sets
   `m.lastUsage`; `/usage` with no prior usage renders "no usage recorded
   yet"; `/usage` after a `Usage`-bearing `EventDone` renders the token
   counts; confirm the existing
   `TestController_Compact_NoManagerSkipsCleanly` is unchanged and stays
   green (it specifically covers the nil-`Compaction` case, still
   reachable whenever no summarizer is configured).
6. **Verify** — go build ./..., go vet ./..., go test ./... across the
   whole module.
