# Unattended RPC cold-start acceptance

Build Hand, then run:

```sh
go build -o /tmp/hand-cold-start ./cmd/hand
python3 scripts/check_rpc_cold_start.py --hand-binary /tmp/hand-cold-start \
  --out /tmp/hand-cold-start-new-run
```

Use a fresh output directory. The runner uses a local HTTP provider fixture and
isolated HOME, with no credentials or terminal. It installs legacy workspace
always-allow settings but no trusted authority. Both process launches must
negotiate and expose approval through RPC. The first explicitly denies a write;
the second creates a distinct session and cancels while approval is pending.
Neither may create the target file or trust store, or change workspace settings.
The provider must receive the denied tool result. Exactly three provider calls
are expected across the two sessions.

Raw JSON exchanges, stderr, provider requests and hashes are retained in the
output directory. A failed assertion, timeout or unsuccessful child cleanup
fails the run. Fresh session operations use unique durable request IDs; reusing
an ID would intentionally replay the earlier result.

This journey supplies development evidence for M4.1 no_hidden_trust_prompt.
Final acceptance still needs exact candidate and released Harness qualification
on the required platforms. See `rpc-cold-start.json` for the recorded macOS run.

The Linux arm64 run is recorded in `rpc-cold-linux.json`. Its container command
is archived in `development-evidence/rpc-cold-linux-20260909/container.json`.
It mounts the Linux Hand binary and runner scripts read-only and writes evidence
to a separate output mount, with external networking disabled. The loopback
provider fixture runs inside the container. The binary hash links to the build
in `rpc-linux-framing.json`; no Go toolchain is required inside the container.
