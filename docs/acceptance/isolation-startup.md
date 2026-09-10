# CLI container execution selection

The staged CLI accepts an `execution` object in the user configuration at `~/.hand/config.json`. Omission or `backend: "host"` preserves unrestricted host execution. Container mode requires absolute Docker executable, Unix socket and Linux worker paths, plus an immutable cached image ID. It does not pull images or fall back to host execution.

```json
{
  "execution": {
    "backend": "container",
    "docker": "/usr/local/bin/docker",
    "socket": "/absolute/path/to/docker.sock",
    "image": "sha256:<64 hex digits from the cached image>",
    "worker": "/absolute/path/to/linux-hand-tool-worker",
    "worker_sha256": "<64 hex digits for the worker binary>",
    "writable": true,
    "network": false
  }
}
```

The image must contain `/bin/bash`. Startup probes both Bash and the worker protocol before constructing the runtime. File/search/skill/todo/web operations route through the worker; shell and background processes use the same selected execution backend. Requests to the model provider remain on the host. Shell commands run in `/workspace`; use workspace-relative paths across tools. The model receives this instruction, including after model/profile switches.

Configured MCP servers and hooks require `trust_external_mcp: true` and `trust_external_hooks: true`, respectively. These are explicit user-level trust decisions: the capabilities execute outside the container and can have host or remote effects. Startup stderr, RPC `hello.execution_boundary`, and TUI `/boundary` describe the effective boundary. Optional MCP servers attached later retain the same disclosed external status.

The Python lifecycle runner now accepts `--execution-config /path/to/config.json` containing just the execution object. It installs that configuration only in the temporary fixture home. A final built CLI passed approval-before-write, steering, queue operations, duplicate handling, cancellation and reconnect recovery with container mode selected and networking disabled. The model provider was a local fixture; no live calls were made. Raw evidence and binary hashes are in `isolation-startup-integration.json`. Configuration and CLI/TUI/RPC race suites passed, followed by the final startup/CLI checks; vet passed.

This remains development work against an unpublished Harness. SDK selection, release packaging, richer credential routing, native Linux-host tests and final acceptance gates remain open. The required worker digest is verified before a private copy is mounted; see `worker-pin.md`. The remaining approval-to-mutation race is also not closed by container selection alone.
