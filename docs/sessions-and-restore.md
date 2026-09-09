# Sessions, checkpoints and recovery

Sessions preserve conversation history. Checkpoints capture selected workspace
files around a run. Selecting an earlier conversation entry does not restore
files; a checkpoint restore is a separate, explicitly reviewed operation.
These instructions describe the development implementation. Final release and
platform qualification remain pending.

## Resume, branch and export a session

Start Hand from the workspace whose history you want. The session catalogue uses
the workspace path, so moving or renaming the directory can change which history
is discovered. Do not rename catalogue files to force a match.

| Command | Effect |
| --- | --- |
| `/resume` | List saved sessions and their stable IDs. |
| `/resume SESSION-ID` | Select a saved session in this workspace. |
| `/name descriptive name` | Name the current session. |
| `/tree` | Inspect the conversation graph. |
| `/tree ENTRY-ID` | Select a conversation entry for subsequent work. |
| `/fork` | Create a separate session from the selected history, preserving the source. |
| `/new` | Start a new session and keep previous sessions. |
| `/clear` | Clear the displayed transcript without deleting saved history. |
| `/export /absolute/new-file.jsonl` | Export the complete session graph to a new path. |

Use the IDs shown by the UI. Wait for active work to settle before switching or
exporting. A busy or writer-ownership error means the operation has not acquired
exclusive access; stop the other owner cleanly instead of deleting lock files.

An export can include attachments. Keep its sibling `.attachments` directory
when copying it. The equivalent model-free CLI export is documented in
[installation and upgrades](installation-and-upgrades.md). Back up the complete
session store before migration or recovery, not just the currently selected log.

## Capture and inspect file changes

Enable checkpoints when starting Hand, using a private absolute directory outside
the workspace:

```sh
hand --checkpoint-dir /absolute/private/checkpoints
```

Choose a checkpoint directory for this workspace and reuse it to inspect its
recorded runs. Use `--checkpoint-exclude` for an exact workspace-relative path or
subtree, for example `--checkpoint-exclude node_modules`. Size, entry and storage
limits are configurable through the `--checkpoint-max-*` flags shown in
`hand --help`. Omitted content is outside the captured scope.

After a run, `/changes` shows recorded file additions, removals and changes.
`/changes RUN-ID` selects a run; follow the displayed next-page command for
additional files. The comparison reports captured before/after bytes and modes.
It does not include later edits or prove that every listed change came from Hand.

## Restore one reviewed file

Use a run ID from `/changes` and a path from its captured scope:

```text
/restore-preview RUN-ID "path with spaces/file.go"
```

Double-quoted paths use JSON/Go-style escapes; no shell expansion occurs. The UI
previews one exact path at a time. Review the proposed operation and any conflict.
If the preview is conflict-free, it prints `/restore-confirm CURRENT-DIGEST`.
Copy that exact command to apply it, or use `/restore-cancel` to discard it.

Confirmation is bound to the displayed preview. If current files changed, obtain
and review a new preview. Do not keep retrying an old confirmation. A restore can
report attempted files and retained recovery paths even when it returns an
error; inspect those results before deciding what to do next.

Restoring a captured file does not reverse deployments, network requests,
database changes or arbitrary shell effects. It does not rewind the conversation
or establish that the resulting workspace passes its tests.

## Inspect an interrupted restore

Restart Hand with the same checkpoint directory, then run `/recoveries`. Keep
any retained recovery files. The UI identifies records and only offers a
resolution when its inspected state permits one:

- `prepared`: the displayed `cancel` resolution closes the preparation record.
- `applied_unrecorded`: the displayed `acknowledge` resolution records the
  observed application.
- Conflict or uncertain state: inspect the target and retained recovery copy;
  no automatic retry is offered.

Run the exact `/recovery-resolve ACTION RECOVERY-ID` command shown on the current
page, then inspect `/recoveries` again. Resolution records the reviewed state
and preserves target and recovery files. It is not a command to overwrite a
conflicting file or delete the retained copy.

## Verify the resulting workspace

With checkpoints enabled, load explicit named verification commands using
`--verification-config /absolute/verification.json`. For example, a Go project
can use this JSON after replacing the evidence-directory path:

```json
{
  "directory": "/absolute/private/verification-evidence",
  "profiles": [
    {"name": "tests", "command": ["go", "test", "./..."]}
  ]
}
```

Commands are argument arrays; shell syntax requires an explicitly selected shell.
Loading the configuration does not execute it. `/verify tests` displays the
command for review. Use its exact `/verify-confirm PROFILE DIGEST` command to run
it. Inspect exit status, output, captured scope and omissions. `/verify-list`
lists saved evidence; `/verify-check PROFILE EVIDENCE-ID` reassesses a saved result
against current files and configuration. A historical pass cannot establish that
later edits pass. Only the actual checks run support a verification claim.

Verification configuration is supported in interactive and RPC modes, requires
checkpoints, and is not accepted with one-shot `-p`. For programmatic session,
checkpoint and verification operations, see the [RPC protocol](protocol/v1.md).

Usage totals include recorded attempts across session branches. If earlier history or an attempt has no usage report, the status line labels the reported subtotal `incomplete`; it is a lower bound, not the full session consumption. `/usage` provides the accounting details. A failed usage read after compaction also marks the cached subtotal incomplete.

For RPC restore, use the `run_id` returned by `checkpoint.changes`; it is the durable checkpoint association, not the execution ID from a terminal event. Inspect `checkpoint.restore` result fields `completed`, `error` and `files` even when the RPC call itself succeeds. The response preserves partial-operation details, so successful transport alone does not establish a successful restore.
