# Staged bounded JSONL export

`/export <new JSONL path>` now exports the full current session in the isolated Hand checkout. The remainder of the command is the path, preserving internal spaces. Export uses the session worker and retains UI identity, transcript and counters. Active runs or background compaction prevent exporting a changing session.

The controller reserves ownership. The manager verifies the session ID/key against the workspace catalogue, flushes the writer and checks that the source is a regular file with stable identity across opening. It copies the durable bytes unchanged, including the format header, all branches and selection controls. Limits are 256 MiB total and Harness MaxSessionRecordBytes (10 MiB) per record, including newline bytes. Copy buffering is 64 KiB; cancellation is checked between chunks and before publication. Over-limit input and short writes fail explicitly.

A private 0600 temporary file is written and synced in the destination directory. An exclusive hard link publishes it, so existing files or symlinks are not overwritten. The containing directory is synced before success. A directory-sync error after publication explicitly reports that the destination was published; it does not imply rollback. Ordinary failed copies remove their temporary files.

Tests reopen an export with a path containing spaces through Harness and verify retained session identity, selected leaf and graph entries, private permissions, unchanged UI identity and refusal to overwrite. Copy tests cover exact size boundaries, aggregate and per-record rejection, records spanning several buffers, byte preservation, pre-cancelled context and short writes. An initial test incorrectly assumed spaces were forbidden in store keys; it was corrected to reopen the export directly, and its failure log is preserved.

Fresh validation passed 537 full uncached race-suite tests/subtests with no failures/skips. Export tests passed 20 repetitions (140 passing test/subtest events), and vet exited 0. Commands ran with GOCACHE=/private/tmp/hand-review-gocache and GOPROXY=off in `/private/tmp/hand-session-integration-20260908`:

```sh
go test -race -count=1 -json -coverprofile=/private/tmp/hand-session-export-coverage-20260908.out ./...
go test -race -count=20 -json ./internal/sessionio ./internal/tui -run 'TestSessionExport'
go vet ./...
```

The aggregate patch includes prior staged session work; source and raw artifact hashes are in `session-export-integration.json`. It excludes the temporary module replacement and preserves older patch snapshots. Primary Hand is still on released Harness v0.3.9 and has not received these changes; the isolated checkout uses unpublished candidate 5cc93a3. No full requirement group is closed.

Remaining qualification: actual export syscall failures and process interruption, cleanup of export temporaries left by a killed process, attachment portability and persisted usage semantics, preventing oversized inline payloads at ingestion, native PTY/platform journeys and released Harness integration. Export preserves existing bytes; it does not yet package external attachments or enforce a bound on total historical session storage. Concurrent writes outside the cooperative writer-lease protocol are outside this snapshot guarantee.
