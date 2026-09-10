# Real OS write-failure development evidence

Added permanent fork/export tests using a disposable child process with a 1,024-byte soft RLIMIT_FSIZE. The child ignores SIGXFSZ so the kernel returns EFBIG from the actual write. Source history is larger than the limit and is created before lowering it. Only the child's soft limit changes; the user's shell and parent test process are unaffected.

For both fork and export, tests require errors.Is(err, syscall.EFBIG), no output files in the export directory, no partial fork discovered after reconciliation, unchanged source bytes and unchanged catalogue selection. The child restores its prior limit and successfully exports the source, checking writer-lease release and retry integrity. This verifies real write failure rather than a callback-generated error. It does not simulate ENOSPC, filesystem permission denial or hardware power loss; those distinctions remain explicit.

Fresh staged full uncached race/coverage validation passed 552 tests/subtests with zero failures/skips. Two operations passed 20 repetitions (40 real write-limit trials; 60 passing parent/subtest events). Vet exited 0. Commands ran in `/private/tmp/hand-session-integration-20260908` with GOCACHE=/private/tmp/hand-review-gocache and GOPROXY=off:

```sh
go test -race -count=1 -json -coverprofile=/private/tmp/hand-session-write-fault-coverage-20260908.out ./...
go test -race -count=20 -json ./internal/sessionio -run TestSessionOperationsHandleRealWriteLimit
go vet ./...
```

Tests are built for Darwin/Linux; this evidence is from native macOS, not Linux runtime qualification. The aggregate patch includes previous staged session changes and excludes the temporary module replacement. Source/raw hashes are in `session-write-fault-integration.json`; prior snapshots remain preserved. Primary Hand still uses released Harness v0.3.9 and has not received the staged patch; development uses unpublished candidate 5cc93a3. No full requirement group is closed.

Required remaining work includes explicit ENOSPC/permission failure scenarios, export temporary recovery after process death, persisted usage/attachment semantics, native platform/PTY qualification, released Harness integration and the other M0–M8 obligations.
