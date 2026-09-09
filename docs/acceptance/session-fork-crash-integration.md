# Staged fork interruption evidence

The staged manager now has a private per-instance fault-boundary callback, unset in production. Tests stop execution after a copied entry, before hard-link publication, after publication, and before catalogue registration. This makes interruption placement deterministic without replacing the real filesystem or Harness persistence.

At each boundary a separate child test process performs the actual fork, signals readiness through a pipe and waits. The parent kills and joins it, reconciles twice, verifies no duplicate registration and checks that the source file is byte-for-byte unchanged. Before publication no fork is discovered. After publication exactly one complete two-entry fork is recovered, its dead writer lease can be acquired and a new continuation can be appended/flushed. Catalogue selection remains on the source during reconciliation.

A second suite returns an injected error at each boundary and verifies that the original error remains visible, failed forks do not select a new session, and recovery finds only complete published copies. This suite validates error handling at transaction boundaries; it is not an actual ENOSPC/permission-denied syscall simulation. Neither suite proves hardware power-loss durability.

Fresh staged validation:

- Full uncached race/coverage suite: 527 tests/subtests passed, zero failures/skips.
- Process-kill suite: 20 repetitions at four boundaries, 80 killed-child trials; 100 passing test/subtest events including parent tests.
- Error-return suite: 20 repetitions at four boundaries, 80 injected-error trials; 100 passing test/subtest events including parent tests.
- Vet: exit 0.

Commands ran in `/private/tmp/hand-session-integration-20260908` with GOCACHE=/private/tmp/hand-review-gocache and GOPROXY=off:

```sh
go test -race -count=1 -json -coverprofile=/private/tmp/hand-fork-crash-coverage-20260908.out ./...
go test -race -count=20 -json ./internal/sessionio -run 'TestForkProcessDeathPublicationBoundaries'
go test -race -count=20 -json ./internal/sessionio -run 'TestForkBoundaryErrors'
go vet ./...
```

The aggregate patch and source/raw artifact hashes are in `session-fork-crash-integration.patch/json`. Earlier snapshots remain preserved. The aggregate includes prior session work, excludes the temporary go.mod replacement, and passes application checking against primary Hand. Primary Hand still uses released Harness v0.3.9 and has not received the staged patch; local validation uses unpublished candidate 5cc93a3.

Bounded abandoned-staging cleanup, syscall-specific disk faults, attachment/usage restoration, export, native Linux and PTY qualification, and released Harness integration remain required. No full requirement or scenario group is closed by these macOS development results.
