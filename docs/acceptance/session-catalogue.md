# Workspace session catalogue groundwork

The internal/sessionio Catalogue stores a version-1 workspace index under a canonical workspace hash. Workspace aliases resolve through symlinks; the canonical path is also checked in the document to detect a hash collision or misplaced index. Relative store roots are anchored on opening.

Records bind stable session IDs to agent/storage keys and contain a name, creation time and last-active time. Register preserves existing sessions and is idempotent for an existing identity/binding. Select records the last active session; Rename changes only its name. Registration requires the application to have already created and validated the durable backend session. Catalogue metadata alone is not proof that the backend exists or that migration succeeded.

Mutations take a nonblocking kernel lock shared by catalogue instances/processes. They validate a fresh snapshot, write a private temporary file, sync it, atomically replace the index and sync the directory. Complete post-rename state may be visible even if the final sync fails; callers must reconcile errors, not assume rollback. Linux/macOS are the advertised runtime targets. The catalogue cannot silently overwrite corrupt, future-version, mismatched-workspace or symlink metadata. Bounds are 4 MiB, 10,000 session records and 256-byte printable UTF-8 names.

Permanent file tests cover preserving old sessions, selection/name restart, idempotent registration, canonical aliases, anchored roots, conflicting identities/storage keys, private modes, corruption and symlink rejection. A real child-process lock/death test demonstrates kernel lock release and successful subsequent registration without losing earlier records. Twenty race-enabled sessionio repetitions passed 260 tests/subtests.

Hand's full local runner passed 482 tests/subtests with zero failures/skips, build/vet/format checks, runner/checker unit suites and binary smoke checks. Overall statement coverage is 85.78%; changed-code coverage is 86.32%, below the required 90%. The earlier runner attempt rejected an uncompiled fallback for unadvertised platforms; that unnecessary new file was removed and fresh evidence collected without changing coverage rules. Both attempts are retained. Hashed source/evidence references are in catalogue-development.json.

The catalogue is not yet connected to main.go or Controller.NewSession: those current paths still select the legacy workspace key and discard history for a new-session request. Their replacement must create a fresh durable Harness session, register only after durable creation, retain/import the legacy session with its backup, and switch the active runtime only after the new selection is ready. Resume must check stored backend identity and acquire its lifetime lease. Interrupted backend creation versus catalogue registration needs reconciliation so orphaned valid sessions are discoverable and repeated migration does not duplicate them. Full UI/CLI new/resume/name/fork/tree/export flows, broader sync/disk-full fault coverage, backend integrity checks and native-platform qualification remain open.

Hand's go.mod still pins released Harness v0.3.9. Development runs now integrate the newer local Harness candidate through go.work; this does not establish released-version qualification. This document records earlier catalogue slices. Consult progress.json and its linked evidence for current development results; no final acceptance group is closed here.

## Durability follow-on

Idempotent registration now syncs the existing index and its directory before reporting success. This repairs a false-success path after an earlier rename committed but directory sync failed. An unchanged index is not by itself proof that the previous update became durable. Error paths retain the complete previous index before rename, expose complete-but-not-confirmed-durable state after rename, and release the catalogue lock.

Added file-sync and rename fault injection, directory-sync/idempotent retry tests, generation exhaustion and size/session-count limits. Actual subprocess tests terminate a writer immediately before or after rename, then verify a complete old/new index and successful nonduplicating restart. Kill-before-rename may leave a hidden staging file; automatic orphan-staging cleanup remains pending. These tests establish filesystem/process effects for their stated boundaries, not physical power-loss behaviour or full backend/catalogue transactional reconciliation.

Fresh full Hand validation passed 492 tests/subtests with zero failures/skips; 20 race-enabled sessionio repetitions passed 460 tests/subtests. Overall coverage is 85.95%; changed coverage is 86.57%, below 90%. The session package cross-compiles for linux/amd64, linux/arm64 and darwin/amd64; only darwin/arm64 execution is evidenced here. Raw hashes and the report location are in catalogue-durability.json.

The earlier release request for candidate 5cc93a3 is historical; its patch and scope predate subsequent local hardening. A fresh release review must use the current Harness candidate and cumulative changes. No released-version qualification is claimed here.

## Branch record integrity

Harness now rejects duplicate top-level record fields and case aliases of known
graph fields, including `parentId`, before restoring the graph. Unknown legacy
extension fields remain supported; opaque nested tool data retains its own
schema. Branch-selection data must contain exactly the supported `version`
field. Ambiguous branch controls are corruption, not repairable tail truncation,
and recovery preserves their file bytes. See harness-selection-integrity.json
for regression, integration and fuzz evidence from the local Harness candidate.
