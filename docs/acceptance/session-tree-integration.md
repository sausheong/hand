# Staged session tree navigation

The isolated Hand checkout `/private/tmp/hand-session-integration-20260908` now exposes `/tree` and `/tree <entry ID>`. Listing shows each conversation node, its parent ID, type/role and the selected-node marker. Header and selection control records are excluded. Node selection uses Harness Branch persistence, retains other graph branches, invalidates queued goal continuations, increments the UI generation and replays the selected history. Operations use the asynchronous session worker; active runs and background compaction prevent selection. Cancelled or invalid selection does not replace the current UI identity.

The controller reserves application ownership while taking a tree snapshot or selecting a node. This avoids a foreground append changing the selected leaf during the snapshot. Current-session writer ownership remains held. Selection success is reported after the Harness persistence primitive succeeds; cancellation after that commit does not restore an obsolete view.

Two permanent TUI/controller tests verify topology/selected marker, metadata listing retaining identity, selected transcript excluding the other branch, selection surviving close/open, the other branch remaining available, control-record exclusion, invalid/cancelled selection and active-run guards. These tests use real session files but run in one process. They complement the prior Harness restart/compaction tests; they are not a native binary PTY or process-crash qualification.

Fresh development validation passed 514 tests/subtests in the full uncached race/coverage suite, with zero failures/skips. The two tree tests passed 20 repetitions (40 passes). Vet exited 0. Commands ran in the isolated checkout with GOCACHE=/private/tmp/hand-review-gocache and GOPROXY=off:

```sh
go test -race -count=1 -json -coverprofile=/private/tmp/hand-session-tree-coverage-20260908.out ./...
go test -race -count=20 -json ./internal/tui -run 'TestSessionTree'
go vet ./...
```

`session-tree-integration.json` preserves source and raw artifact hashes. The aggregate patch includes prior session changes and supersedes earlier snapshots for application; their historical evidence is retained. The patch excludes the isolated go.mod replacement. Primary Hand is still on released Harness v0.3.9; this checkout uses unpublished candidate 5cc93a3 and cannot satisfy released-dependency acceptance.

Remaining M3.1 work includes `/fork` into a separate session, bounded JSONL export and attachment/usage restoration, combined recovery cases, background shutdown joins, primary integration and native end-to-end qualification. Tree display uses explicit parent links; a navigable structured viewer with reflow/performance qualification remains M3.3 work. No requirement group is closed by this development evidence.
