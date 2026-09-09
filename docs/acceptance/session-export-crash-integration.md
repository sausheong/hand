# Staged JSONL export interruption checks

A private per-manager callback, unset in production, exposes deterministic boundaries immediately before and after exclusive destination publication. The process test starts a separate exporter against a saved real session, waits for the selected boundary, kills and joins the child, then inspects the destination and original source bytes. Before publication the destination is absent; afterwards it is byte-identical to the source. A subsequent export succeeds, proving the killed process no longer owns the source writer lease.

Error-return tests at the same boundaries verify that the original error remains identifiable, ordinary cleanup removes temporary files, a post-publication error explicitly reports publication, and ExportID releases its writer on failure. These are transaction-boundary error injections, not actual ENOSPC or hardware power-loss tests. The real-process tests leave killed-process temporary files for fixture cleanup; production recovery of those temporaries remains outstanding.

Fresh staged full uncached race/coverage validation passed 548 tests/subtests with zero failures/skips. Two-boundary process termination passed 20 repetitions: 40 killed-child trials, 60 passing test/subtest events including parent tests. Vet exited 0. Commands ran in `/private/tmp/hand-session-integration-20260908` with GOCACHE=/private/tmp/hand-review-gocache and GOPROXY=off:

```sh
go test -race -count=1 -json -coverprofile=/private/tmp/hand-export-crash-coverage-20260908.out ./...
go test -race -count=20 -json ./internal/sessionio -run TestExportProcessDeath
go vet ./...
```

The aggregate patch and source/raw artifact hashes are in `session-export-crash-integration.patch/json`; earlier snapshots remain preserved. The patch excludes the temporary local module replacement and passes application checking against primary Hand. Primary Hand remains on released Harness v0.3.9 with the staged session patch unapplied; development uses unpublished Harness 5cc93a3. No full requirement group is closed.

Remaining work includes killed-export temporary cleanup, syscall-specific disk failure coverage, persistence and portability of usage/attachments, native Linux and PTY qualification, released Harness integration and the other full-plan obligations.
