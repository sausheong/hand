# Harness legacy compaction migration

Local candidate `4c78cf45c5439e34f7b2789d1d1c9dc85ddab618`, based on `44309157ae4563839a19cf60bd57492cbbd3cbce`. Unpublished and not integrated into Hand (Harness v0.3.9). The earlier release request does not cover this follow-on.

## Implementation

ConvertLegacySession validates a complete legacy JSONL stream, preserving original nodes and converting the historical compactor's repeated nodes to deterministic unique graph IDs. Repeated nodes must have identical content, including unknown fields, and follow the compaction clone-chain pattern. Conflicting or unrelated duplicates, missing/forward parents, invalid selections, malformed records and truncated tails are rejected. Known compaction range references are mapped to the appropriate node versions; self/missing range references are rejected. Tool-call identities in data remain unchanged.

The converter retains unknown JSON fields and integer precision. Logs without repeated IDs remain byte-identical; repeated conversion of converted output makes no change. IDs derive from the complete original source digest and record position, and are checked against source IDs. Limits are 128 MiB input, 100,000 records, the existing 10 MiB record limit, and 256 MiB converted output. Conversion is bounded but keeps the source and parsed data in memory; it is not a streaming constant-memory importer.

Store.MigrateLegacy acquires the lifetime writer lease, validates/converts, writes a private complete-original backup, syncs it and its directory, then atomically replaces the session and syncs the directory again. A returned session retains the writer lease until Close. The report includes the source digest, conversion counts, backup path and whether replacement occurred. A later sync error remains an error even if replacement is visible. Ordinary valid logs are not rewritten or backed up again. Truncated-tail recovery is a separate reported operation rather than an implicit part of conversion.

## Validation

The local full race/coverage suite passed 862 tests/subtests, zero failures/skips. Session tests passed 2300 tests/subtests across 20 race-enabled repetitions. FuzzLegacyConversion passed 60 requested seconds (61.02 elapsed, 4,395,062 executions), asserting that successful output is accepted by the strict production decoder and is idempotent. Session/compaction/runtime vet and whitespace checks passed.

Permanent regressions cover graph identity, all original node versions, unknown fields/large integers, deterministic/idempotent conversion, range remapping, record-count bounds and corruption rejection. File tests verify private backups, restart, selected-branch continuation, writer exclusion, sync failure and cancellation. Actual subprocesses are killed at the durable-backup and durable-replacement boundaries, then restart, verify backup bytes and append successfully. An initial subprocess-test compile error caused by a helper name was corrected; the failed build and fixed run are retained separately from final passing evidence. Artifact hashes are in harness-migration.json.

## Remaining scope

This is the legacy log-format primitive for M3.1, not the complete workspace-hash-to-multiple-session catalogue import. Canonical workspace identity, durable session IDs, session names/last-active selection, complete schema-version enforcement, attachment/export bounds, released dependency integration and native-platform acceptance remain required. Backup files are retained for recovery; there is no automatic destructive retention policy. Kernel leases coordinate participating writers and do not prevent arbitrary external filesystem mutation. Process-kill tests do not simulate hardware power loss.
