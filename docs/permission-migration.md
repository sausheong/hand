# Permission migration and journal recovery

Hand stores persistent permission decisions outside the workspace, under
`~/.hand/authority/<workspace-hash>/<configuration-digest>/grants.jsonl`.
The workspace is canonicalised; effective configuration changes select a different
authority context. Keep the same workspace, configuration and model flags when
inspecting or changing a particular context.

## Review legacy settings

From the workspace, run:

```sh
hand --permissions
```

This prints `grants`, `legacy_proposal`, `legacy_fingerprint` and `legacy_warning`.
It does not call a model or require provider credentials. Inspection may create
an empty private authority journal. Entries in `.hand/settings.json` are proposals
only; they do not silently become persistent authority.

Review the complete proposal before importing it. Legacy grants permit a tool
broadly, without the narrower resource scope of a fresh approval. To import a
reviewed proposal, copy its exact fingerprint:

```sh
hand --permissions --ack-legacy-permissions REVIEWED_FINGERPRINT
```

An incorrect fingerprint fails without granting authority. Repeating the same
acknowledgement does not duplicate existing grants. Import persists one grant at
a time, so an interrupted import can leave a subset imported; repeating the
reviewed acknowledgement resumes it. Re-review if the workspace, effective
configuration or legacy tool list changes.

To revoke a grant, copy its `ID` from the inspection output:

```sh
hand --permissions --revoke-permission GRANT_ID
hand --permissions
```

Revocation survives restart. Do not repeat a legacy import after revocation unless
you intend to reauthorise that proposal. `--yes` is a separate broad approval for
one invocation; it does not create persistent grants.

## Recover from rejected authority storage

Startup fails when the journal is malformed, truncated, ambiguous, too large,
not private, or refers to an active resource whose canonical location changed.
Duplicate JSON fields, case aliases, null fields and missing grant fields are
rejected. Current Hand serialisation writes the required canonical fields.
A failed replay does not publish partial grants or rewrite the rejected journal.

The startup diagnostic identifies the exact authority directory. Stop all Hand
processes using that workspace and configuration before touching it. If the
failure is exclusive ownership, close the existing owner and retry; do not move
an actively owned directory.

Preserve the entire affected directory before recovery. For a malformed journal
without an independently verified valid backup, move that directory to a unique
private quarantine location outside every coding workspace. Rename only the
specific directory identified in the diagnostic, not the whole authority root.
Do not overwrite an earlier backup. Record its original path and retain its bytes
for diagnosis. Do not try to recover permissions by deleting individual records:
removing a revocation can revive a grant.

Run `hand --permissions` again with the same configuration. Hand creates a fresh
empty authority context. Verify `grants` is empty, then approve future operations
individually or explicitly review and acknowledge a legacy proposal. Keep the
quarantined directory; do not copy its rejected records into the fresh journal.

If a saved active resource changed through a symlink, inspect that change before
reapproval. A new approval may refer to a different resource from the old one.
An execution sandbox remains necessary to restrict arbitrary host code running
as the same operating-system user.

## Rollback limits

Keep the previous executable and an untouched backup of its data before upgrading.
Do not assume an older binary understands new sessions, journals or revocations.
Test rollback with a separate HOME and a copy of the pre-upgrade data. Do not run
old and new binaries concurrently against the same authority journal. Reverting
the executable does not reverse workspace file changes; use a reviewed checkpoint
or version-control recovery for those changes.

This guide covers permission migration and recovery. Complete release rollback,
session-format migration and exact release compatibility still require the final
candidate acceptance evidence.

## Reproduce the acceptance journey

```sh
go build -o /tmp/hand-permission-migration ./cmd/hand
python3 scripts/check_permission_migration.py \
  --hand-binary /tmp/hand-permission-migration \
  --out /tmp/hand-permission-migration-new-run
```

Use a fresh output directory. The runner creates an isolated HOME and workspace,
checks inspection, rejected acknowledgement, explicit import, idempotent import,
revocation and restart. It also restores each nonempty proper record prefix of
a three-grant import and verifies that resume adds only missing grants without
rewriting earlier records. It then injects a malformed journal. It verifies the diagnostic
path, preservation of rejected bytes and an empty authority after quarantine.
The fixture acknowledges only its own generated test proposal. It does not alter
personal permissions. Raw stdout, stderr, exit codes and hashes are retained.

The partial-import check uses authentic CLI-written record prefixes. It verifies
recovery semantics at durable boundaries and does not simulate an actual process
kill or establish crash safety during a partially written record.

## Routine-work policy and prompt counts

Workspace reads are allowed by the default scoped hook. New file writes and shell
commands require a decision. Choosing persistent approval authorises the exact
file-write resource or exact shell command. A repeated edit to that file can reuse
the write grant; a different file requires its own decision. A changed shell
command requires its own decision. External reads and network destinations remain
explicit capabilities. An approved host command can execute arbitrary code; exact
command matching does not confine what that code does.

The development RPC fixture performs two writes to `note.txt` and two identical
shell checks with two explicit persistent approvals. It verifies the final file
contents and two check outputs. A different file, an expanded shell command, a
network fetch and an external read each prompt separately and are denied. This
small workflow passed on macOS and Linux; it is not a claim about all workflows.

```sh
python3 scripts/check_permission_prompts.py --hand-binary /tmp/hand \
  --out /tmp/hand-permission-prompts-new-run
```

Use a freshly built Hand binary and a fresh output directory. The runner creates
an isolated home/workspace and loopback provider fixture. Raw protocol exchanges,
provider requests, prompt counts and hashes are retained. See
`docs/acceptance/permission-prompt-count.json` for the recorded development runs.

A separate process-interruption runner exercises the real importer:

```sh
python3 scripts/check_permission_interrupt.py --hand-binary /tmp/hand \
  --out /tmp/hand-permission-interrupt-new-run
```

It creates 256 test-only legacy proposals in an isolated HOME, starts the built
CLI import, observes a nonempty journal, and stops/kills that child process.
A new process must inspect and resume the partial import, retain its prefix and
avoid duplicate records on repeated acknowledgement. It fails if the importer
finishes before interruption or if no partial import survives. It never kills
personal Hand processes. Raw interrupted and recovered journals are retained.
The recorded macOS and Linux runs passed; these are process-crash checks, not
power-loss simulation or exhaustive write/fsync-boundary testing.

## Allow all Bash commands directly

In interactive Hand, run:

```text
/permissions allow bash --project
```

This explicitly allows all Bash commands for the current project and effective
configuration, including future sessions. Repeating it does not add duplicate
grants. Run it when Hand is idle. No `settings.json` or legacy acknowledgement is
needed. Changing the model or configuration may select a different authority.

Inspect permissions with `/permissions`. To remove this grant:

```text
/permissions revoke project-bash
```

Revocation removes this grant immediately for future approval checks; it does
not stop executing commands or remove other grants (including exact-command or
legacy grants). Those remain visible and can be revoked by ID.

Project scope identifies where the approval applies; it does not restrict a
shell command's filesystem or network access. The execution backend controls
those boundaries. This grant does not approve other tools.

The approval prompt's **always this command** choice only remembers that exact
Bash command. Use the explicit project command above for all Bash commands.
