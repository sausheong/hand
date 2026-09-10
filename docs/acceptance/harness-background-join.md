# Run-owned background compaction

Local unpublished Harness candidate `0581441bef89c7e88bcceab17e49b7a7dcca2f4f` adds context-aware compaction, cancellation followed by producer joining, and a final session flush before successful EventDone. The final usage aggregate includes observed background attempts. Cancellation during the join emits EventAborted. Stop-hook work runs before finalisation. ForgetSession cancels and joins an existing producer before dropping session locks.

The legacy detached API remains available; its consumers must separately own and join their producers. Joining may add summarisation latency to run completion and requires cooperative provider cancellation. Fast-completed producer result/error retention still needs review; this slice does not close the complete compaction requirement.

The new regression holds the summariser after cancellation, checks that stream closure waits, and asserts final event ordering, usage totals and cancelled outcomes. It passed 20 race-enabled repetitions (60 test/subtest passes). Full uncached race suites passed 881 Harness and 566 Hand tests/subtests, zero failures/skips; both vet runs passed. The initial regression fixture omitted Tools and panicked; that failure is preserved and the fixture was corrected before collecting passing evidence.

[Patch](harness-background-join.patch) and [hashed evidence](harness-background-join.json). Hand uses the unchanged sources identified by [the previous integration manifest](session-compaction-shutdown-integration.json), revalidated against both primary and staged files. Its temporary local dependency replacement is development-only. Primary Hand still pins Harness v0.3.9; no release was published. Coverage, platform, live evaluation and full acceptance remain pending.
