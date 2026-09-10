# Parent-side built-in tool routing

`internal/toolproxy.Tool` preserves the registered tool's definition and concurrency contract, but replaces execution with a request to `/hand-worker` through an explicit backend. It never invokes the original tool's host Execute method. Approval remains the caller's responsibility.

The wrapper bounds requests to the worker protocol limit and captures responses up to 16 MiB. It rejects backend errors, nonzero exits, truncation, missing results, incorrect versions and trailing JSON. Failures after dispatch are described as potentially completed operations; there is no automatic retry or host fallback. The worker's explicit image payload is restored to the result, and metadata includes the effective execution boundary.

Harness Container now accepts a trusted regular executable mounted read-only at `/hand-worker`. Capture size remains 64 KiB by default; protocol callers can explicitly request up to 16 MiB. The worker response limit includes its terminating newline.

Tests cover large responses/images, invalid response forms and backend failure using a host implementation that panics if executed. Native tests use the real Container backend and Linux worker to write and read a temporary workspace, transfer a response larger than 64 KiB and fail on a missing worker. Affected proxy/worker and execution/shell race suites and vet passed. Evidence and worker hash are in `tool-proxy-integration.json`; Harness changes are in `harness-worker-routing.patch`.

This is a staged integration component, not an available Hand isolation mode yet. Startup configuration, mapping approved host workspace paths to container paths, boundary reporting, background/MCP/extension routing, packaging and final platform acceptance remain open. Native evidence is Docker Desktop on macOS, not Linux-host qualification.
