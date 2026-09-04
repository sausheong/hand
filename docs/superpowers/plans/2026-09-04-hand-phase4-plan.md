# hand Phase 4 (One-shot mode + polish) Implementation Plan

**Goal:** `-p` non-interactive mode consulting Phase 2's allowlist, plus
diff/command previews on the approval prompt.

**Spec:** [docs/superpowers/specs/2026-09-04-hand-phase4-design.md](../specs/2026-09-04-hand-phase4-design.md)

## Tasks

1. **`internal/agentio/diff.go`** — `lineDiff`, `buildPreview`. Unit
   tests per the spec's Testing section.
2. **`ApprovalRequest.Preview` + `NewApprovalHook(..., workspace string)`
   + `NewOneShotApprovalHook`** — wire `buildPreview` into the existing
   hook; new one-shot hook. Update `approval_test.go`'s call sites for
   the new parameter; add one-shot hook tests.
3. **`internal/tui` approval panel** — preview rendering (colored by line
   prefix), dynamic viewport-height layout in `refreshViewport()`
   replacing the fixed math in `resize()`. Update/extend
   `model_test.go`'s approval tests for the preview text.
4. **`cmd/hand/main.go`** — `-p`/`--yes` flags, one-shot branch
   (`runOneShot`), `NewApprovalHook`'s new `workspace` argument at the
   interactive call site.
5. **Verify** — `go build ./...`, `go vet ./...`, `go test ./...` across
   the whole module.
