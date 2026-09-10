# Harness durable branch selection and compaction

Local candidate `44309157ae4563839a19cf60bd57492cbbd3cbce`, based on `46dfa7b3f20b3e16189610c7a7ef44276c102689`. Unpublished and not integrated into Hand (still Harness v0.3.9). This follow-on is outside the earlier release request's candidate.

## Behaviour

Branch selection is now a locked, synced JSONL control record of type `selection`, with the target in `parentId` and data `{"version":1}`. It survives restart and Rewrite and travels with a copied/exported session file. Control records appear in Entries but never in History/View; they cannot be selected as conversation nodes. Appends after selection use the selected conversation node as parent. Branch returns persistence errors and does not report a successful in-memory selection when its write/sync failed; a post-write sync failure can still have changed disk state and requires reconciliation through the existing sticky persistence-error mechanism.

Legacy Compact now creates a separate summary branch and fresh clones of preserved messages. It retains every original record, ID and parent edge. Automatic compaction previously re-appended duplicate IDs and changed the original graph; it now calls CommitCompaction to atomically install summary plus clones, preserving tool-call IDs/content inside message data. A stale selected leaf rejects the compaction snapshot with ErrSessionChanged.

Strict decoding rejects empty/duplicate IDs, missing/forward/control-record parents, cycles implied by invalid append order and unsupported selection-record versions. Invalid graph records are not recoverable truncated tails. Generic Append rejects invalid graph nodes and selection control injection without mutating the graph, recording a visible sticky error.

## Evidence

Permanent tests exercise selected-branch restart and Rewrite, subsequent parentage, original branch restoration after legacy/automatic compaction, stale compaction, disk write failure, concurrent branch/append/view operations and graph corruption. The two base-compatible regressions fail on `46dfa7b3f20b3e16189610c7a7ef44276c102689` and pass on the candidate. The initial strict-graph test run exposed the automatic compaction duplicate-ID defect; the implementation was fixed before collecting the final passing results. Both the failing observation and fixed run are retained.

The full local race/coverage suite passed 838 tests/subtests, zero failures/skips. Session and compaction suites passed 3,040 tests/subtests across 20 race-enabled repetitions. FuzzSessionRecords passed 60 requested seconds (61.04 elapsed, 423,691 executions). Session/compaction/runtime vet and whitespace checks passed. Artifact paths and hashes are in harness-branches.json.

## Remaining work

This advances M3.1 selected_leaf_persisted and branch_restart_compaction but closes neither integrated acceptance scenario. Older automatic-compaction logs can contain duplicate IDs: strict loading now rejects them rather than silently rewriting graph identity. A backed-up, versioned legacy import must reconcile these records before Hand integration. Full record schema/version policy, session catalogue/migration, durable session identity, attachment/export limits and native-platform acceptance remain open. Selection records are versioned, but that alone is not a complete versioned session format. Whole-log compaction replacement is atomic and preserves history; performance and bounded storage still need their acceptance evidence.
