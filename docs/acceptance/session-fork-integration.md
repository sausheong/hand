# Staged session fork command

`/fork` in the isolated Hand checkout `/private/tmp/hand-session-integration-20260908` copies the currently selected history into a separate session and selects it. Use `/tree <entry ID>` first to choose another fork point. The new session has a distinct durable ID and catalogue entry named “Forked session”; entry IDs and tool-call payload references remain scoped to the copied session. Source history, unselected branches and selected leaf remain unchanged. Subsequent messages extend the fork independently.

The controller reserves application ownership, rejects background compaction, flushes the source and retains its writer until the new fork is durably created and selected. The TUI uses the cancellable session worker, invalidates queued continuations, increments generation and replays the copied history. Failed creation leaves the source selected.

Manager.Fork creates a private staging directory under the store root, outside the discoverable session namespace. Harness writes/validates the copied entries there, flushes and closes them. An exclusive hard link publishes the complete file under a random workspace session key. The manager reopens/validates that file, flushes its directory entry and only then registers/selects it in the catalogue. An error after publication can leave a complete recoverable orphan. An error before publication removes the staging directory during ordinary unwinding; process-killed staging directories are currently retained outside discovery and need bounded cleanup qualification. The source is never deleted or rewritten.

Permanent tests check distinct fork identity, selected-history-only replay, source graph/selection preservation, independent continuation surviving close/open, duplicate-entry failure publishing no partial copy, pre-cancelled context rejection, unchanged catalogue selection after failed fork/reconciliation and ordinary staging cleanup.

Fresh development evidence: 516 full uncached race-suite tests/subtests passed, zero failures/skips; two fork tests passed 20 repetitions (40 passes); vet exited 0. Commands ran in the isolated checkout with GOCACHE=/private/tmp/hand-review-gocache and GOPROXY=off:

```sh
go test -race -count=1 -json -coverprofile=/private/tmp/hand-session-fork-coverage-20260908.out ./...
go test -race -count=20 -json ./internal/tui ./internal/sessionio -run 'TestSessionFork|TestForkFailure'
go vet ./...
```

Source/artifact hashes are in `session-fork-integration.json`. The aggregate patch includes previous session changes, excludes the development go.mod replacement and preserves earlier snapshots. Primary Hand remains on released Harness v0.3.9 and has not received the patch; development uses unpublished Harness 5cc93a3. No full scenario or requirement group is closed.

Remaining qualification includes process termination and disk faults at fork publication boundaries, resource bounds and abandoned staging cleanup, external attachment references and persisted usage semantics, bounded JSONL export, native PTY journeys and released Harness integration. The current tests are real-file in-process tests, not proof of power-loss or process-crash recovery. The branch copy uses typed Harness records; preservation of unknown future fields is not established.
