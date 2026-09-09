# Harness atomic rewrite follow-on — local evidence

Commit `c6c155d314f176527d0dd3e33253d592bf82ef61` follows persistence candidate `ec89b4d718182ddde190c036907522a166f7e7d6` in `/private/tmp/hand-harness-persistence-20260908`. Neither follow-on is published or integrated into Hand. The original 308f702 checkout remains unchanged.

Previously, Rewrite truncated the destination before serializing every replacement entry. The new regression reproduces loss of the original file when a later entry cannot be encoded; its failing log is preserved. Rewrite now creates a private temporary file in the destination directory, encodes all entries, flushes/syncs/closes it, then atomically renames it and syncs the directory. Failures before rename preserve the original; errors after rename remain persistence failures and require reconciliation even though a complete replacement may be visible. Temporary files from ordinary error returns are removed.

Rewrite now uses the same session/write/store lock order as Append. Compaction locks its in-memory rebuild and keeps subsequent disk appends behind the replacement snapshot. This fixes concurrent mutation and ordering gaps without claiming the legacy Compact method preserves all branches; complete DAG/selected-leaf semantics remain separate work.

The session suite passed 20 race-enabled repetitions, 700 tests/subtests, including concurrent append/rewrite and append/compaction comparisons between memory and reloaded disk. Fresh full candidate race/coverage validation passed 782 tests/subtests, zero failures/skips. Session/runtime vet passed. The incremental patch is harness-rewrite.patch; raw evidence locations and digests are in harness-rewrite.json.

Process termination during rewrite, stale temporary-file recovery, writer leases, directory-creation durability, complete branch/session migration and all deferred/background persistence paths still need implementation and qualification. Native Linux/hosted release checks and publication authorisation are absent. Hand remains on released Harness v0.3.9; these local results do not close M3.1 or any full acceptance group.
