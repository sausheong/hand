# Container worker packaging and verification

Packaging is implemented by `scripts/build_dist.py` and is now called by
`make dist`. Both validation and release workflows require worker verification.
The earlier integration patch is retained as historical evidence; do not apply
it again. The current checkout uses a local Harness development dependency and
has not been qualified as a release candidate.

See [installation and upgrades](../installation-and-upgrades.md) for checksum,
installation, backup and rollback instructions.

Each archive contains the native `hand` CLI, `hand-tool-worker-linux`,
`WORKER.json`, README and licence. The worker is Linux ELF even in a macOS
archive because it executes inside a Linux container. Its architecture matches
the archive architecture. Remote Docker engines with a different architecture
are not covered by this packaging contract.

Extract the complete archive to a trusted directory outside the workspace.
Set execution.worker to the absolute worker path and execution.worker_sha256
to the SHA-256 in WORKER.json. Configure the explicit Docker executable,
local daemon socket and immutable cached image ID as described in the
isolation integration documentation. The image must contain Bash. Startup
verifies and privately snapshots the worker before mounting it read-only.
The manifest is an integrity reference, not an independent signature; obtain
the archive and checksum through the trusted distribution channel.

`build_dist.py` requires a new output directory and never removes an existing
one. A failed build leaves partial output for inspection; it does not produce
SHA256SUMS until all four archives have been built.

Run `scripts/smoke_dist.py` with `--require-worker` to check all archive hashes,
worker executable modes, actual ELF architectures and manifest identities.
The release workflow patch enables this strict mode. The native CLI smoke
runs only on the current host architecture; other targets are explicitly
reported as not executed.

For the integrated local journey:

```sh
python3 scripts/check_packaged_isolation.py \
  --archive /absolute/path/hand-VERSION-darwin-arm64.tar.gz \
  --version VERSION --commit FULL_COMMIT_SHA \
  --execution-config /absolute/path/local-container-fixture.json \
  --out /absolute/path/new-evidence-directory
```

This runner extracts only the named regular executables, checks the CLI identity
and worker manifest, and exercises the RPC lifecycle through a local provider
fixture with networking disabled in the worker container. It records archive,
worker and runner hashes, the RPC evidence and process output. It does not call
a real model or download an image. Use the archive matching the current host.

The September 8 development run passed nine RPC scenarios with five provider
fixture calls and two cancelled provider requests. This verifies the packaged
macOS ARM64 CLI and Linux ARM64 worker together. Native Linux-host execution,
AMD64 runtime journeys, final candidate qualification and live evaluations
remain outstanding. See `packaged-isolation.json` for raw evidence references.
