# Completed compaction result retention

Local unpublished Harness `3b7fe5ec6536b7bd629e690037c2e5116a2d60cb` fixes a race where an async producer removed its result before its owner joined. A stale-session compaction error could previously disappear and the runtime would emit successful EventDone.

Context-owned producers now retain one result per session until JoinInFlight or ForgetSession consumes it. HasInFlight reports only active execution. WaitForInFlight observes without consuming, and repeated launches cannot overwrite an unread result. The legacy detached API retains its existing automatic cleanup behaviour. Owners must join even when HasInFlight is false.

Both regressions failed on the preceding implementation. The runtime regression forces a real CommitCompaction stale-view error and waits for the producer to finish before finalisation; it now observes ErrSessionChanged and no EventDone. Manager tests cover completed success, result consumption, observation and subsequent launch. Final targeted checks passed 20 consecutive race-enabled repetitions (100 test/subtest passes).

Full uncached race suites passed 883 Harness and 566 Hand tests/subtests with zero failures/skips; both vet runs passed. After full suites, the runtime assertion was strengthened to require the precise stale-session error and the targeted suite was rerun. No production behaviour changed after full-suite validation.

[Patch](harness-compaction-result.patch), [hashed evidence](harness-compaction-result.json). Staged Hand sources match the previously recorded session integration manifest and were revalidated. Primary Hand remains on released Harness v0.3.9. These are development checks, not final release qualification or closure of the complete milestone.
