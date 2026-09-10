# Staged fork staging cleanup

Fork copies now retain a per-directory OS owner lock until ordinary cleanup. A short store-root lock protects directory creation and owner-lock acquisition from concurrent cleanup. Reconciliation inspects at most 256 store-root entries and removes at most 32 unowned `.fork-` directories per pass. Busy copies are preserved; a busy root lock defers cleanup to another pass. Symlink directories are ignored. Liveness comes from kernel locks, not PID or age guesses. A missing store is a no-op, allowing first-session creation.

Permanent tests verify live owner protection, abandoned-directory removal, symlink target preservation, per-pass removal limits, cancelled cleanup and first-session creation. The existing real-process kill tests continue to exercise reconciliation after each fork boundary.

The first full run exposed a fresh-install regression: acquiring the cleanup lock failed when the store directory did not exist. Fixed this before rerunning, added the permanent first-session regression and preserved the failing raw log in the manifest. Fresh validation passed 530 tests/subtests with zero failures/skips. Targeted cleanup and process-death tests passed 20 repetitions (160 passing events, including 80 killed-child trials). Vet exited 0.

Commands ran in `/private/tmp/hand-session-integration-20260908` with GOCACHE=/private/tmp/hand-review-gocache and GOPROXY=off:

```sh
go test -race -count=1 -json -coverprofile=/private/tmp/hand-fork-staging-fixed-coverage-20260908.out ./...
go test -race -count=20 -json ./internal/sessionio -run 'TestForkStaging|TestForkProcessDeath'
go vet ./...
```

The aggregate `session-fork-staging-integration.patch` includes previous session work, excludes the temporary go.mod replacement and preserves previous snapshots. Source and raw evidence hashes are in the adjacent JSON manifest. Primary Hand remains unchanged by this integration and uses released Harness v0.3.9; development uses unpublished candidate 5cc93a3. No full requirement group is closed.

Limits: removal counts bound ordinary cleanup work, not filesystem syscall duration or the contents of externally modified directories. Only the first 256 directory entries are examined per pass, so a crowded unrelated store root can delay cleanup beyond that window. Total staging bytes and parser/export record bounds, syscall-specific disk faults, attachment/usage persistence, export, native platform/PTY qualification and released-dependency integration remain outstanding.
