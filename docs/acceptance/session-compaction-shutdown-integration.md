# Staged manual-compaction shutdown joining

Manual compaction now registers a worker and Done channel when the command is created. A dispatch gate preserves the existing Bubble Tea contract: provider work begins only when the returned command runs. Cancellation is checked after dispatch, so cancelling a queued command never starts a provider call. The result channel is buffered and does not require a still-running UI to consume it.

CloseApplication cancels and joins this worker before session cleanup. Update also joins it before releasing the compaction guard and applying accounting. This closes the terminal-disconnect path that previously had no explicit manual worker join. Session identity and generation checks still reject stale results.

New tests close the application before command dispatch and assert the provider is never called. A second test dispatches a held provider, starts shutdown, observes cancellation, verifies shutdown remains blocked until the provider is released, then verifies the command and compaction operation both finish. Existing cancellation-before-dispatch, stale-result, quit/join, transcript-preservation and accounting tests remain unchanged and pass.

Fresh full uncached race/coverage validation passed 566 tests/subtests with zero failures/skips. The selected shutdown/manual-compaction tests passed 20 repetitions (160 passing events), and vet exited 0. Commands ran in `/private/tmp/hand-session-integration-20260908` with GOCACHE=/private/tmp/hand-review-gocache and GOPROXY=off:

```sh
go test -race -count=1 -json -coverprofile=/private/tmp/hand-compaction-shutdown-coverage-20260908.out ./...
go test -race -count=20 -json ./internal/tui -run 'TestCompactionShutdown|TestManualCompaction|TestCancelledManualCompaction'
go vet ./...
```

The aggregate patch and source/raw hashes are in `session-compaction-shutdown-integration.patch/json`. Prior snapshots are preserved and the temporary local dependency replacement is excluded. Primary Hand remains on released Harness v0.3.9 with the staged session integration unapplied; development uses unpublished c16dccd. No full requirement group is closed.

This is in-process model/controller lifecycle evidence, not a native PTY terminal-disconnect test. Background automatic compaction still needs its own producer-join contract; legacy accounting uncertainty, attachment portability, native/platform qualification and released Harness integration remain pending.
