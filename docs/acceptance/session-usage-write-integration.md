# Usage journal write validation

Usage recording now validates existing records and prospective totals before mutation. Identical request retries do not append duplicates; conflicting IDs, unsupported/malformed journals and aggregate overflow return errors without poisoning persisted accounting. Foreground and background compaction callbacks share one mutex through validation and append.

New tests verify unchanged durable bytes and correct totals after reopen, rejection of invalid existing journals, and 16 simultaneous provider completions whose totals would overflow. The new write regressions fail against the previous source, reconstructed and hash-verified from the prior aggregate patch.

The full race/coverage suite passed 594 tests/subtests with zero failures/skips; vet passed. Usage checks and the concurrency regression each passed 20 repetitions. Raw logs, coverage, prior-source overlays and hashes are stored in [the manifest](session-usage-write-integration.json). [The aggregate patch](session-usage-write-integration.patch) excludes the temporary dependency replacement.

The suspected empty-selected-branch accounting gap was ruled out by Harness graph validation; no speculative branch change was made. This integration is staged against unpublished Harness b2433fa; primary Hand remains on v0.3.9. Full acceptance and large-session accounting performance remain pending.
