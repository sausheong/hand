# hand Phase 2 (Permissions) Implementation Plan

**Goal:** Persisted "always allow" per tool name via `.hand/settings.json`.

**Spec:** [docs/superpowers/specs/2026-09-04-hand-phase2-design.md](../specs/2026-09-04-hand-phase2-design.md)

Implemented directly (TDD, one commit per task) rather than via fresh
subagent dispatch, since the phase is small and the implementer already
holds full context of the Phase 1 codebase this builds on.

## Tasks

1. **`internal/permissions` package** — `Settings`, `DefaultPath`, `Load`
   (no auto-create), `Save`, `IsAlwaysAllowed`, and `Store` (load-once +
   mutex-guarded `IsAlwaysAllowed`/`SetAlwaysAllow`). Unit tests per the
   spec's Testing section.
2. **`agentio.ApprovalRequest`/`NewApprovalHook`** — `Respond` becomes
   `chan Decision` (`DecisionDeny`/`DecisionOnce`/`DecisionAlways`);
   `NewApprovalHook` takes a nil-safe `*permissions.Store` second
   parameter; short-circuits on `IsAlwaysAllowed`; persists on
   `DecisionAlways`. Update `approval_test.go` for the new type.
3. **`internal/tui` three-way prompt** — `handleKey`'s pending branch
   grows an `a`/`A` case; `statusLine` prompt text lists all three
   options. Update `model_test.go`'s approval tests for `chan
   agentio.Decision`; add an "always" case.
4. **`cmd/hand/main.go` wiring** — construct the `permissions.Store` at
   `permissions.DefaultPath(workspace)`, pass into `NewApprovalHook`.
5. **Verify** — `go build ./...`, `go vet ./...`, `go test ./...` across
   the whole module; manual smoke check of the three-way prompt.
