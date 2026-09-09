# Complete session listing metadata

Harness previously stopped scanning records above 1 MiB and ignored scanner errors, presenting partial counts and timestamps as complete. Listing now permits bounded legacy image records and reports per-file `MetadataError` for malformed JSON, read/scan failures and non-regular files. On failure the key remains discoverable for explicit recovery, counts/identity are cleared, and file timestamps are used when available. Listing remains syntactic metadata extraction, not graph validation.

Tests cover a 2 MiB legacy record followed by another message, damaged final JSON, symlinks, injected reader failure and record limits. Twenty race-enabled repetitions passed (40 test events). Full Harness and staged Hand race suites passed 910 and 588 tests/subtests respectively, with zero failures/skips; both vet checks passed.

[Patch](harness-list-metadata.patch) and [hashed raw evidence](harness-list-metadata.json) record the unpublished candidate and current Hand source manifest. Primary Hand still uses released Harness v0.3.9. Full milestone acceptance remains pending.
