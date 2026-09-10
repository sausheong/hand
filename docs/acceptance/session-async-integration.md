# Staged asynchronous session switching

The isolated checkout `/private/tmp/hand-session-integration-20260908` now runs `/new` and `/resume <ID>` selection and history replay outside the TUI event loop. `session-async-integration.patch` is an aggregate patch against the primary Hand source and includes the prior manager, CLI and session UI changes. It supersedes the earlier patches for application; their original artifacts and evidence remain preserved. The patch passes `git apply --check` against the current primary tree and excludes the development go.mod replacement.

Primary Hand remains on released Harness v0.3.9 and has not received this patch. The isolated checkout uses local Harness candidate `5cc93a3d6a1a504cd348d435db8ebd7e68050d28`. Publication permission remains pending. These results cannot satisfy the final released-dependency or clean-candidate gates.

## Behaviour and regression coverage

Session changes immediately invalidate queued goal continuations. A generation-tagged worker owns selection and replay, with a buffered terminal result and an explicit completion channel. Ctrl+C cancels the operation; prompt submission and conflicting commands remain guarded until the worker has joined. Quit requests cancellation and exits after completion. Application shutdown cancels and joins the worker even if Bubble Tea no longer consumes its result. Stale generations cannot replace the selected identity.

A selection failure retains the current UI/backend identity. If selection has already committed when cancellation arrives, the result applies the committed identity and reports interrupted replay instead of restoring an obsolete identity. Successful switching resets transient usage and timing as in the prior integration. Controller context variants propagate cancellation to session creation/opening. New-session compatibility handling rejects sticky persistence errors and avoids flushing an empty, never-written legacy session.

Permanent tests cover cancellation retaining the operation guard, blocked prompt submission, stale results, shutdown joining a delayed worker, late cancellation preserving consistent identity, and failed selection preserving identity. Existing queued-goal tests still assert invalidation before awaiting the command, then join the asynchronous worker before fixture cleanup. The first run exposed that missing test join as a TempDir cleanup failure; its raw log is preserved in the manifest. The implementation/test correction preceded the fresh passing runs.

## Development validation

- Full uncached race/coverage suite: 508 tests/subtests passed, zero failures or skips.
- Targeted session-switch and queued-goal regressions: 20 repetitions, 140 tests/subtests passed, zero failures or skips.
- `go vet ./...`: exit 0.
- The full suite includes the existing three-process compiled CLI new/resume journey. The new interactive tests exercise the model/controller in process; they do not qualify binary PTY journeys.

Commands ran in the isolated checkout with `GOCACHE=/private/tmp/hand-review-gocache GOPROXY=off`:

```sh
go test -race -count=1 -json -coverprofile=/private/tmp/hand-session-async-coverage-20260908.out ./...
go test -race -count=20 -json ./internal/tui -run 'TestSessionChange|TestSessionAndModelChangesInvalidateQueuedGoal'
go vet ./...
```

Source before/after hashes and raw output hashes are in `session-async-integration.json`. The coverage profile is preserved; no new claim of meeting changed-code or critical-subsystem coverage is made.

## Remaining acceptance work

Session listing and naming still perform synchronous I/O. Replay captures the starting width/style; resize-aware structured reflow remains M3.3 work. Cancellation is observed between records, not inside a single Markdown render or an already-running filesystem syscall. Large-session latency and 30-trial cancellation performance remain unmeasured. Persisted usage and attachments, fork/tree/export, background-compaction shutdown joins, combined corruption/migration recovery, native platform and PTY qualification, and integration with released Harness remain required. No requirement group or full scenario set is closed by this evidence.
