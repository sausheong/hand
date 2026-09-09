# Staged asynchronous session metadata commands

The isolated Hand checkout `/private/tmp/hand-session-integration-20260908` now runs session listing (`/resume` without an ID) and naming (`/name`) outside the TUI event loop. They share selection's generation checks, operation guard, cancellation and shutdown joining. Metadata results append to the current transcript without replacing the session identity or resetting usage/timing counters. Naming reports a successful committed write even if cancellation arrives afterwards; cancellation cannot undo an atomic filesystem commit. Listing checks cancellation before/after catalogue loading and between rendered records. Naming checks cancellation before the catalogue mutation.

The new aggregate `session-metadata-integration.patch` includes all prior staged session work and supersedes earlier patches for application. Earlier snapshots remain preserved. Primary Hand still uses released Harness v0.3.9 and has not received the patch. The isolated checkout uses the unpublished local Harness candidate 5cc93a3; the module replacement is excluded from the patch. Publication permission remains pending.

Validation in the isolated checkout used GOCACHE=/private/tmp/hand-review-gocache and GOPROXY=off:

```sh
go test -race -count=1 -json -coverprofile=/private/tmp/hand-session-metadata-coverage-20260908.out ./...
go test -race -count=20 -json ./internal/tui -run 'TestSessionChange|TestSessionMetadata|TestSessionCommands'
go vet ./...
```

The full suite passed 512 tests/subtests with no failures or skips. Targeted checks passed 20 repetitions, 200 tests/subtests with no failures or skips. Vet exited 0. An earlier sandboxed app test run could not bind its local HTTP listener; the failure log is retained. The fresh full suite ran with local fixture access. New assertions verify metadata commands retain identity, transcript and usage counters and that a cancelled context cannot mutate catalogue generation. Existing tests verify exact persisted names and selected-session listing. Source and artifact hashes are recorded in `session-metadata-integration.json`.

This is development evidence, not final qualification. Filesystem calls already in progress are not interruptible; catalogue locks and OS latency are not a measured cancellation-performance guarantee. Structured transcript rendering/reflow, large-list key latency, native binary PTY journeys, persisted usage/attachments, fork/tree/export, complete recovery and shutdown joins, and released Harness integration remain required. No requirement group is closed.
