# agcode Phase 3 (Sessions) Implementation Plan

**Goal:** Persist sessions via harness's `session.Store`, one per workspace
directory, auto-resumed on start; `--new-session` resets.

**Spec:** [docs/superpowers/specs/2026-09-04-agcode-phase3-design.md](../specs/2026-09-04-agcode-phase3-design.md)

## Tasks

1. **`internal/sessionio` package** — `StoreDir()`, `KeyForWorkspace()`.
   Unit tests: stability, uniqueness across paths, valid single path
   component.
2. **`internal/tui.ReplayHistory`** — pure function turning
   `[]session.SessionEntry` into styled transcript lines; `(*Model)
   LoadHistory`. Unit tests per `EntryType`/role.
3. **`cmd/agcode/main.go` wiring** — `--new-session` flag,
   `session.NewStore`/`Load`/optional `Delete`, `m.LoadHistory` call.
4. **Verify** — `go build ./...`, `go vet ./...`, `go test ./...` across
   the whole module.
