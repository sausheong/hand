# Troubleshooting

Record `hand --version`, the exact command and working directory, the error,
exit status and whether execution used the host or container backend. For RPC,
retain the request ID, session ID and relevant response/terminal event. Remove
credentials and private workspace content before sharing diagnostics. Version
output alone does not prove a dirty development checkout matches a release.

## Startup, models and permissions

| Symptom | Next step |
| --- | --- |
| Invalid invocation or configuration; exit 2 | Check `hand --help` and the configuration field named in the error. Inspection/export flags cannot be combined arbitrarily with execution flags. |
| Provider credentials or endpoint rejected | Check the selected provider/profile, its credential environment-variable reference and endpoint. Do not print the key. Changing providers must not inherit the old provider's custom endpoint. |
| Unsupported profile output/reasoning settings | Use settings supported by the selected provider and installed Harness version. The current implementation rejects unsupported output configuration rather than silently honouring it. |
| An operation is denied in one-shot or RPC mode | Review effective scoped permissions. One-shot cannot receive approval replies. RPC must handle `approval.pending` and explicitly respond using current IDs. |
| Legacy broad grants are listed but not effective | Review the proposal and fingerprint using the [permission migration guide](permission-migration.md). A workspace's old settings do not authorise themselves. |
| Approval authority cannot open or its journal is corrupt | Preserve the directory identified by the error. Follow the guide's quarantine/recovery procedure after all owners stop. Do not delete individual grant or revocation records. |

`--yes` is a broad per-invocation approval, not a repair for a trust-store error.
A provider/runtime failure returns exit 5; preserve the original error rather
than repeatedly changing settings until an unrelated request happens to pass.

## Sessions, checkpoints and verification

| Symptom | Next step |
| --- | --- |
| Expected session is missing | Confirm the workspace's absolute path and selected HOME/store. Use `/resume`; moving a workspace can change its catalogue identity. |
| Session/store is busy | Stop the owning process cleanly and wait for shutdown. Do not remove locks to admit a second writer. |
| Unsupported future schema or malformed saved state | Preserve the full store and attachments. Use a compatible binary or the pre-upgrade backup; do not downgrade the only copy. |
| `/changes` is empty | Check that checkpoints were enabled before the run, then inspect captured scope and omissions. Later edits are not part of the recorded before/after comparison. |
| Restore preview conflicts or confirmation is stale | Inspect current files and create a fresh preview. The old confirmation does not authorise overwriting later changes. |
| Restore failed after some work | Retain reported recovery paths, inspect `/recoveries`, and use only the resolution offered for the inspected state. |
| Verification previously passed but is no longer current | Run `/verify-check` with the profile and evidence ID, then review and rerun the relevant verification if files/configuration changed. |

See [sessions and restore](sessions-and-restore.md) for exact commands and
[installation and upgrades](installation-and-upgrades.md) for complete backups
and rollback. A completed answer and a historical test pass are different facts.

## Container, package and extension failures

Start with `/boundary` to inspect effective execution isolation. Container
configuration requires explicit absolute Docker/socket/worker paths and immutable
image/worker digests. Confirm the local daemon is available and the cached image
matches the selected architecture. A permission-denied socket is an operating
system/daemon access issue; changing Hand's scoped grants will not fix it.

For a worker digest mismatch, compare the installed worker with the selected
archive's `WORKER.json`. Review an upgraded worker's path and digest together.
Do not disable verification or substitute an unrelated executable. A macOS archive
contains a Linux worker intentionally: the worker executes in a Linux container.

For package integrity or compatibility failures, inspect the package pin, manifest,
Hand version and lockfile through `hand packages inspect`, `show` and `list`.
Prepare a new review when the source or installation changes. Installing a package
does not activate its extensions. Interpreted extensions also need a separately
approved runtime review. A missing interpreter or rejected container admission
must not trigger fallback to unrestricted host execution.

See [packages and extensions](extensions.md) and [package skills](package-skills.md).
Extension stdout must contain protocol frames only; send diagnostics to stderr.
A blocked structured question requires the client to present and answer it, not
an invented default approval.

## RPC disconnects and uncertain work

A malformed or oversized frame can close the connection. Verify newline-delimited
UTF-8 JSON and the one-megabyte frame bound in the [protocol](protocol/v1.md).
Negotiate `hello` on each connection. Keep diagnostics separate from stdout.

On transport failure, reconnect and query the original request ID in the same
session. `request_uncertain` means the retained operation needs reconciliation;
it is not permission to execute it again under a new ID. Event cursor gaps mean
progress was evicted; use durable request lookup for the terminal result. Restart
connection-local cursors at zero after reconnecting.

The [SDK guide](automation-and-sdk.md) explains the difference between requesting
run cancellation and cancelling a call context, which closes the connection.
Close the client and inspect cleanup errors before opening another writer.

## Usage, budgets and evaluation claims

Unknown usage is not zero. Check `unknown_requests` and `prior_unknown`, including
on historical records. Route tariffs are caller-supplied assertions rather than
provider billing statements. Missing/stale prices can block strict budget
admission; inspect `/prices` and `/cost` before approving new tariffs or limits.

Development test results are not a release certificate or a comparison with Pi.
The [evaluation guide](acceptance/evaluation/README.md) provides reproducible
manifest, source and fixture commands. The current calibration catalogue is
smaller than the required qualification corpus. Its qualification check is
expected to report incomplete inputs; do not remove requirements to make it pass.
Paid calls require separate budget authorisation. Final completion also requires
fresh evidence for a clean candidate and the full acceptance checker.
