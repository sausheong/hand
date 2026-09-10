# Development checkpoint capture

The staged implementation supports explicit checkpoint capture around a whole
application run (including its continuation iterations).

CLI:

```sh
hand --checkpoint-dir /absolute/private/path/checkpoints
hand --checkpoint-dir /absolute/private/path/checkpoints -p 'Your task'
hand --checkpoint-dir /absolute/private/path/checkpoints --rpc
```

Embedded Go SDK: set `EmbeddedOptions.CheckpointDirectory` to a private directory
outside the workspace. No user configuration is implicitly loaded. Closing the
SDK client joins its run and releases checkpoint ownership so it can reopen.

The directory is created with private permissions when absent. Existing storage
must be private, and only one process can own it at a time. Do not put it in the
workspace, including through a symlink alias. CLI and SDK apply the same default
limits: 10,000 visited entries, 8 MiB per file, 64 MiB captured bytes, directory
depth 64, 100 stored snapshots and 512 MiB encoded storage. Capacity exhaustion
rejects a new capture; it does not automatically delete accepted checkpoints.

Capture retains regular file bytes and permissions. It reports omissions for
symlinks, `.git`, `.hand`, `.ssh`, `.aws`, `.env` and `.env.*`. These defaults do
not identify every possible secret filename. Captured content should be treated
as private workspace data. External effects such as deployment are outside file
rollback. Custom CLI/SDK exclusions and limits are available as described below.

A running Hand background process prevents capture; new background starts are
rejected while capture is in progress. External editors are not locked. Capture
is not an atomic filesystem snapshot and may reject concurrent changes. Before
capture/journal failure prevents the run from starting. After capture failures
are recorded explicitly; they never produce a complete after-image.

Session metadata stores start/finish associations to snapshot hashes. An
interrupted start remains unfinished. Forking or selecting conversation history
does not restore files. Use `/changes` for the latest run or `/changes RUN-ID OFFSET` for a specific
run/page. It shows recorded before/after file metadata in 32-file pages, including
omission counts. This comparison does not include edits made after capture. RPC
clients can call `checkpoint.changes` with `run_id` and `offset`. Inspection
requires an idle application and reports missing/incomplete evidence explicitly.
Use `/restore-preview RUN-ID PATH` to review one exact path, including spaces.
Double-quoted paths support escaped characters; shell expansion is never used.
RPC `checkpoint.restore_preview` accepts `run_id` and an array of up to 32
`paths`. Both report proposed operations/conflicts without changing files.
After reviewing a conflict-free preview, enter `/restore-confirm CURRENT-DIGEST` using the exact digest displayed, or `/restore-cancel` to discard it. Confirmation is consumed once. Later workspace edits require a fresh preview. The terminal displays attempted files and retained recovery paths even on partial failure.

These features remain development integrations requiring the staged Hand patch
and development Harness dependency. They have not passed the full release
acceptance contract. See checkpoint-rpc.json and sdk-checkpoint-integration.json
for local fixture evidence.

### Confirmed RPC restore (development integration)

After obtaining `checkpoint.restore_preview`, present its exact actions and conflicts to the user. On explicit confirmation, send `checkpoint.restore` with a unique request ID and params `{ "confirmed": true, "preview": <the complete preview object> }`. Inspect `result.completed`; RPC transport success alone does not mean the restore completed. `result.files` lists attempted files and retained recovery paths, while `result.error` describes failure. Multi-file changes are sequential and may partially apply. Do not automatically retry failed operations. Reusing the same request ID and payload retrieves its persisted response; an uncertain ledger response requires reconciliation. Stale workspace previews are rejected. Recovery files remain until explicit reconciliation/retirement is implemented.

The built fixture is `python3 scripts/check_rpc_client.py --hand-binary /absolute/path/to/hand --out /new/external/evidence/path --checkpoints --restore`. It uses only a local HTTP provider. A real terminal fixture is available with `python3 scripts/check_changes_terminal.py --hand-binary /absolute/path/to/hand --fixture /completed/rpc/fixture --out /new/terminal/evidence --restore`. Use a fresh RPC fixture without `--restore`; the terminal journey mutates its workspace.

### Configuring capture scope and capacity

Repeat `--checkpoint-exclude PATH` for exact workspace-relative files or subtrees. Paths are literal, not globs. Configure `--checkpoint-max-entries`, `--checkpoint-max-file-bytes`, `--checkpoint-max-total-bytes`, `--checkpoint-max-snapshots` and `--checkpoint-max-store-bytes`; zero uses existing defaults. Limits apply when `--checkpoint-dir` enables capture. Invalid paths and bounds reject startup. Reaching capacity rejects capture without deleting prior snapshots.

For embedded clients, set `EmbeddedOptions.Checkpoints` to `sdk.CheckpointOptions{Exclude: []string{"private"}, MaxFileBytes: 1048576}` alongside `CheckpointDirectory`. The other fields are `MaxEntries`, `MaxTotalBytes`, `MaxSnapshots` and `MaxStoreBytes`. Exclusions are copied during Open. Generated `.hand-restore-<32 lowercase hex>` recovery paths are always omitted and cannot be restored as ordinary project files.

The built CLI custom-scope journey adds `--checkpoint-scope` to the RPC fixture command. Keep the same capture settings across clients if you intend to confirm a preview after restarting.
