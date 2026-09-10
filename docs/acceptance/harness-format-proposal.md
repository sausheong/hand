# Harness versioned session identity

Local candidate `5cc93a3d6a1a504cd348d435db8ebd7e68050d28`, based on `4c78cf45c5439e34f7b2789d1d1c9dc85ddab618`. Unpublished and not integrated into Hand, which still uses Harness v0.3.9. This follow-on is outside the earlier release request's candidate.

## Behaviour and format

New session files begin with a `session_header` carrying their stable ID, schemaVersion 1 and creation timestamp. The header is retained across append, Rewrite and Rename; it is excluded from Entries, History and View. Store.List exposes its ID without counting it as a conversation entry. New-file Create commits a complete synced private header using an exclusive hard link, preserving an existing destination. First append to a missing/empty file writes its header before the message.

The header must be the first nonblank record and cannot be repeated or used as a conversation parent/selection target. Unknown header versions, a misplaced version field and reuse of the header ID as a conversation node are rejected. Load, LoadRecovering and MigrateLegacy do not silently repair or import unsupported complete schemas.

MigrateLegacy now also upgrades headerless files, including valid unique-ID or empty logs. It preserves the original backup before installing the header, reports FormatUpgraded separately from duplicate-ID remapping, and retains historical creation time when available. Repeated migration of a valid version-1 log preserves its exact bytes and session ID. Ordinary legacy Load remains non-mutating and has no durable ID until upgrade. This supersedes the previous migration proposal's statement that all valid headerless logs remain unchanged by MigrateLegacy; the pure ConvertLegacySession still preserves exact bytes when no duplicate-ID conversion is needed.

The committed Harness SESSION_FORMAT.md documents identity, controls, migration and limitations. In particular, serialising Entries alone is not a complete identity-preserving export, and a copied file retains identity until a product fork assigns a new one.

## Evidence

Permanent tests cover stable identity across empty creation, first append, rewrite and rename; List metadata; header exclusion from history; exclusive creation; empty/ordinary legacy upgrades with original backups; repeat-upgrade idempotence; invalid/future header rejection across all read/repair/migration paths; and header-ID collision rejection. Migration subprocess tests now verify the durable header after the replacement boundary.

Full local race/coverage validation passed 875 tests/subtests, zero failures/skips. Session tests passed 2560 tests/subtests across 20 race-enabled repetitions. Both FuzzSessionRecords and FuzzLegacyConversion passed 60 requested seconds; exact execution counts and durations are in harness-format.json. Session/compaction/runtime vet and whitespace checks passed. Test source bytes were unchanged while the local commit was finalised; these are development results, not final release acceptance.

## Remaining scope

The application workspace/session catalogue, named/new/resume/fork flows, payload-specific schema validation, attachment/export limits and released integration remain required. Headerless reads still expose ephemeral IDs until explicit migration. Typed generic rewrites do not yet guarantee preservation of arbitrary unknown fields. Native-platform and integrated acceptance remain pending. No complete requirement group is closed by these local APIs and tests.
