# Verification profiles (development integration)

Create an existing private evidence directory outside the workspace, then an explicit JSON configuration:

```json
{
  "directory": "/absolute/private/verification-evidence",
  "profiles": [
    {"name": "unit", "command": ["go", "test", "./..."]}
  ]
}
```

Start Hand with `--checkpoint-dir /absolute/private/checkpoints --verification-config /path/to/verification.json` in interactive or RPC mode. Configuration does not run commands. Paths relative to the configuration file are supported for the evidence directory. Commands are literal argv; a shell must be selected explicitly. Embedded SDK clients use `EmbeddedOptions.Verification` with `sdk.VerificationOptions` and `sdk.VerificationProfile` alongside checkpoint options.

RPC clients call `verification.profiles` and display the exact command and boundary. After explicit user confirmation, send `verification.run` with a unique request ID and `{ "profile": "unit", "digest": "<displayed digest>", "confirmed": true }`. It immediately returns a ledger record. Poll `request.get` with that ID until its state is `completed`; its `result` is a stored RPC response, whose result contains `verification`, `completed` and optional `error`. Inspect `verification.assessment.status` and command exit/error evidence. Successful RPC transport or operation completion does not mean tests passed.

Use `verification.check` with `{ "profile": "unit", "id": "<saved evidence ID>" }` to reassess against freshly captured workspace state. Later edits or profile changes make results stale; missing snapshot/output evidence makes them unverified. Results describe only captured scope, including explicit omissions. Commands may have external effects that snapshots cannot roll back.

`cancel` cancels active verification. Disconnect cancels and joins it. Reusing the same request ID and payload retrieves the original ledger record without rerunning; unresolved records after a crash require reconciliation. Evidence files are content-addressed but rely on the trusted private directory, not cryptographic authentication.

Run the development journey with `python3 scripts/check_verification_rpc.py --hand-binary /absolute/path/to/hand --out /new/external/evidence/path`. It makes no provider calls. Final qualification remains pending.

## Terminal verification

Use `/verify` to list profiles or `/verify unit` to review one. The display includes literal argv, execution boundary and `/verify-confirm unit DIGEST`. Enter the displayed command only after reviewing it. Confirmation is consumed once; Ctrl+C cancels the joined worker. Results include assessment, exit code, bounded output and saved evidence ID, with errors shown alongside available results.

Use `/verify-check unit EVIDENCE-ID` to reassess later. A prior exit code of zero can coexist with a stale or unverified assessment; only the current evidence-backed assessment describes captured scope. Commands may modify files or external systems.

Run `python3 scripts/check_verification_terminal.py --hand-binary /absolute/path/to/hand --out /new/external/evidence/path` for the real terminal review/confirmation/staleness journey. It makes no provider calls.

## Evidence retention

Optional configuration fields `max_records` and `max_bytes` bound saved verification evidence. Zero/omitted values use 1,000 records and 256 MiB. Maximum supported values are 10,000 records and 8 GiB. The evidence directory uses exclusive writer locking; reaching a limit rejects new evidence without deleting earlier records. Orphan staging bytes count against capacity.

Capacity is checked at publication, so a command can run successfully yet fail to save its evidence. That returns an error, no saved evidence ID and an unverified assessment. Do not treat the command exit code alone as accepted verification. Evidence cleanup remains explicit; automatic eviction is not performed.
