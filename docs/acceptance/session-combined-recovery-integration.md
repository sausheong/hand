# Combined legacy migration and tail recovery

Harness `MigrateRecovering` validates and migrates the retained prefix under one writer lease, including known compaction clones and oversized inline images. It backs up the complete original bytes before atomic replacement. Strict migration remains strict. Reports identify recovered tail bytes only after replacement; corrupt prefixes and newline-terminated malformed records remain errors.

Staged Hand now invokes this combined operation after a recoverable-tail error and retains backup diagnostics on failure. Restart preserves the repaired graph and branch selection without repeated backups.

## Development verification

- Harness full race suite: 908 passing tests/subtests, zero failures/skips; vet passed.
- Hand full race/coverage suite: 588 passing tests/subtests, zero failures/skips; all seven tested packages ended with pass and stderr was empty. The original process handle had expired at final inspection.
- Combined Harness checks passed 20 repetitions (160 events), including 40 killed child processes at backup/replacement durability boundaries.
- Hand recovery checks passed 20 repetitions (40 events). The new manager regression failed against the previous source, verified using the prior manifest hash.
- Hand vet passed. Raw logs, coverage, prior-source overlay and hashes are preserved in the JSON manifest and development-evidence directory.

See [Hand patch](session-combined-recovery-integration.patch), [evidence](session-combined-recovery-integration.json), and [Harness patch](harness-combined-recovery.patch).

## Remaining work

This is staged integration against unpublished Harness 98ac701. Primary Hand still uses v0.3.9. Full requirement coverage, native platforms, released dependency integration, clean candidate qualification and authorised live evaluations remain pending. Passing development tests does not close M3.1 or the goal.
