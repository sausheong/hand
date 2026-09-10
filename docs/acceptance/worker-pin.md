# Worker executable integrity

Container execution requires `worker_sha256`, a lowercase 64-digit SHA-256 digest of the worker binary. It is included in the configuration used to bind persisted grants. A changed expected digest therefore selects a different authority identity; bytes that do not match the configured digest are rejected.

Before mounting the worker, Harness copies verified bytes into its private container state directory outside the workspace mount. It uses a no-follow, nonblocking source open on Unix, rejects nonregular/nonexecutable sources and limits worker size to 128 MiB. Copying checks cancellation and computes the digest over the actual copied bytes. Incomplete snapshots are removed. The immutable copy is mounted read-only and kept until the container lifecycle joins cleanup. Replacing the original pathname after verification does not replace the mounted bytes.

Tests verify source replacement after copying, hash mismatch, removal of failed snapshots, malformed digests and cancellation. Configuration/proxy race tests include actual native container execution from the verified snapshot. Vet passed, and a fresh CLI passed the isolated Python RPC lifecycle with the required digest configured. Evidence is in `worker-pin-integration.json`.

The Docker executable and daemon remain trusted host infrastructure. These snapshots do not defend against arbitrary trusted host code running as the same user. SDK selection, release packaging, atomic file mutation preconditions and final platform/acceptance gates remain pending.
