# Execution backends: first native implementation

The development Harness `execution` package defines a `Backend` interface accepting structured argv, explicit environment and bounded stdin. Its host implementation is labelled unrestricted. Its container implementation uses an absolute Docker executable, a local Unix socket and an immutable cached image ID. It never falls back to host execution or pulls an image.

Container creation uses a read-only root filesystem, dropped capabilities, no-new-privileges, a non-root user, PID/memory/CPU limits, disabled healthchecks and Docker logging, an explicit workspace bind mount and a 64 MiB temporary scratch filesystem. Workspace write access and bridge networking are separate opt-ins. Docker receives an empty temporary configuration directory; parent environment and Docker credentials are not forwarded to the workload. Image-baked environment remains part of the selected image's trust boundary.

The command lifecycle joins Docker creation, starts the named container, and force-removes it after normal completion or cancellation. Creation has a 30-second timeout and cleanup a five-second timeout. Cleanup errors are reported. This does not yet meet the final cancellation latency contract for a hung daemon during creation. Output is limited to 64 KiB per stream with explicit truncation; full artifact capture integration remains pending.

Native development tests on Docker Desktop's Linux arm64 engine used the already cached Alpine image recorded in the manifest. Tests exercised outside-path symlinks, read-only workspace/root writes including child commands, explicit versus inherited environment, scratch writes, absent routes and failed network connectivity, permitted workspace writes and cancellation of a shell with a child. The initial test incorrectly assumed that loopback was the sole interface; Docker Desktop exposes inactive tunnel interfaces. The corrected test checks actual route/connectivity restrictions. Three tests passed under the race detector, no skips, and vet passed. A subsequent container listing found no remaining test containers.

This package is not yet wired into Hand's built-in tools, background processes, MCP or extensions. Those integrations, visible boundary reporting, Linux-host qualification, full artifact capture and final acceptance remain required. The Harness commit is local and unpublished.

Docker option semantics were checked against the [official container run reference](https://docs.docker.com/reference/cli/docker/container/run/).

## Implicit mount hardening

The backend now inspects the immutable image before creating a container and rejects nonempty image volume declarations. Inspection is bounded, cancellable and fail-closed for malformed/truncated metadata. A cached PostgreSQL image with a data volume was inspected and rejected without running its entrypoint. Workspace binds now use `bind-recursive=disabled`; Docker documents that nested mounts are otherwise included by default in its [bind mount reference](https://docs.docker.com/engine/storage/bind-mounts/#recursive-mounts).

Normal native execution and shell tests passed with this option. An adversarial nested-mount test on a native Linux host remains required; acceptance of the option on Docker Desktop alone does not prove that scenario. See `harness-execution-mounts.json` for the recorded development evidence.
