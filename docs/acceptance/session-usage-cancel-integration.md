# Staged terminal accounting after cancellation

Service execution now admits the backend's final session_usage snapshot after cancellation while continuing to suppress late text and other progress. Stream captures that accounting before its cancellation/drop checks and attaches it to the terminal event, retaining the same value through FinalEvent after Done closes. This avoids requiring space in a full progress queue to deliver final accounting.

The TUI reads the immutable terminal snapshot after stream.Wait and applies usage before finishing the run. The existing active-stream identity check still rejects stale results. Late model output remains suppressed; applying accounting does not turn a cancelled outcome into a completed one.

Permanent tests fill the bounded progress queue without draining, cancel, require the worker to finish, verify the outcome remains cancelled and compare terminal channel/FinalEvent accounting values. A TUI application test cancels before delayed output and verifies final totals/unknown counts update while the late text is absent.

Fresh staged full uncached race/coverage validation passed 562 tests/subtests with zero failures/skips. Both cancellation regressions passed 20 repetitions (40 passes), and vet exited 0. Commands ran in `/private/tmp/hand-session-integration-20260908` with GOCACHE=/private/tmp/hand-review-gocache and GOPROXY=off:

```sh
go test -race -count=1 -json -coverprofile=/private/tmp/hand-usage-cancel-coverage-20260908.out ./...
go test -race -count=20 -json ./internal/app ./internal/tui -run 'TestCancelledFullQueue|TestCancelledApplication'
go vet ./...
```

The aggregate patch and source/raw artifact hashes are in `session-usage-cancel-integration.patch/json`; previous snapshots remain preserved. Primary Hand still uses released Harness v0.3.9 with the staged session integration unapplied. Development uses unpublished Harness c16dccd; the local replacement is excluded from the patch. No full requirement group is closed.

This resolves cancelled-run display delivery for a backend snapshot emitted before its stream closes. Background compaction still needs producer joining to guarantee all accounting/errors are included; manual compaction accounting, legacy uncertainty, attachments, native qualification and released Harness integration remain pending.
