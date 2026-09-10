# Staged manual compaction accounting

Extracted a shared application usage observer that chains Harness observers, persists request annotations and retains the first recording error under a mutex. Foreground runs use it as before. Controller.Compact now installs it on the owned manual-compaction context and joins any recording error into its returned error after the compaction call finishes.

The asynchronous TUI compaction command reads a durable accounting snapshot in its worker. Its result carries reported totals and unknown-attempt counts; Update applies those after checking operation generation and session identity, including when the compaction was cancelled. Accounting-read failures are shown. A skipped compaction that makes no provider request adds no usage record.

Permanent tests run a reported-usage summarisation, require a committed compaction, verify displayed totals and reopen the real session file to confirm the usage remains durable. A cancellation test waits for the summariser call, cancels it and verifies an unavailable-usage attempt remains counted in the UI.

Fresh staged full uncached race/coverage validation passed 564 tests/subtests with zero failures/skips. Both new tests passed 20 repetitions (40 passing events), and vet exited 0. Commands ran in `/private/tmp/hand-session-integration-20260908` with GOCACHE=/private/tmp/hand-review-gocache and GOPROXY=off:

```sh
go test -race -count=1 -json -coverprofile=/private/tmp/hand-manual-usage-coverage-20260908.out ./...
go test -race -count=20 -json ./internal/tui -run 'TestManualCompactionUsage|TestCancelledManualCompactionRetains'
go vet ./...
```

The aggregate patch and source/raw hashes are in `session-manual-usage-integration.patch/json`; all changed staged Go files were checked for inclusion and earlier snapshots are preserved. The patch excludes the temporary local replacement. Primary Hand remains on released Harness v0.3.9 with the staged session integration unapplied; development uses unpublished c16dccd. No full requirement group is closed.

Background compaction producer joining, terminal disconnect while manual compaction is active, legacy usage uncertainty, per-provider pricing, attachment persistence/portability, native/platform qualification and released Harness integration remain pending. These deterministic fixture results are not live provider billing evidence.
