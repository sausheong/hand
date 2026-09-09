# Durable session annotations: local integration foundation

Harness local candidate `c16dccd1770c14165cab442732f7e38a3c37ee6b` adds Session.Annotate and Session.Annotations. An annotation is versioned application metadata with a kind and JSON payload, limited to 64 KiB encoded data. It persists and flushes without moving the selected leaf, including in an empty session. It remains in full Entries/rewrites/raw exports and is excluded from model History/View. It cannot be selected as a branch or used as a conversation parent. Annotations returns independent payload copies. Invalid input returns an error without poisoning persistence; write failures retain the existing degraded-persistence contract.

The Hand tree adapter now excludes annotation controls, with a permanent regression verifying the selected conversation node is unchanged. This is the storage primitive for future usage, outcome and model-change records; Hand does not yet persist request usage or restore totals. Fork inheritance/accounting semantics and application payload schemas remain to implement.

Harness validation: 878 full uncached race-suite tests/subtests passed, zero failures/skips; three annotation tests passed 20 repetitions (60 events); vet passed. Parser fuzzing ran 61.480 seconds with 1,494,037 executions and passed. Tests cover empty/nonempty leaf preservation, restart, compaction retention, invalid/future input, mutable getter isolation, concurrent message appends and closed writers. The incremental Harness patch and raw hashes are in `harness-annotations.patch/json`. Harness checkout is clean at the local candidate commit; nothing was published.

The first concurrent Hand validation hit the existing viewport timing threshold (1.285 seconds against 1 second) while parser fuzzing was active. The failed log is preserved. Fuzzing was allowed to finish, then the full Hand suite was run separately with the threshold and viewport implementation unchanged. It passed 553 tests/subtests with zero failures/skips; Hand vet passed. This resolves the execution contention for this development run; it is not the final M3.3 performance qualification.

Hand commands ran in `/private/tmp/hand-session-integration-20260908` with GOCACHE=/private/tmp/hand-review-gocache and GOPROXY=off:

```sh
go test -race -count=1 -json -coverprofile=/private/tmp/hand-annotation-isolated-coverage-20260908.out ./...
go vet ./...
```

The aggregate Hand patch and hashes are in `session-annotation-integration.patch/json`. It includes earlier staged session work and excludes the temporary module replacement. Previous artifacts preserve their original dependency/source snapshots. Primary Hand remains on released Harness v0.3.9 and has not received this integration.

The prior release proposal/permission request identifies 5cc93a3. It is not approval to publish this newer candidate. The annotation format extension is unpublished: the earlier 5cc93a3 development loader does not understand annotation control semantics and must not open files produced by this candidate. Before publication, refresh the concrete release proposal and native qualification against the final candidate. No full requirement group is closed. Usage/attachment integration, platform and fault qualifications, released Harness integration and all other plan obligations remain pending.
