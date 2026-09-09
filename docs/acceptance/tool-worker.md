# Private built-in tool worker

`cmd/hand-tool-worker` is an execution helper for an already-admitted operation. Its parent must run it within the selected backend. Running it directly on the host grants no isolation; it deliberately provides neither an approval service nor a network listener.

It reads one JSON request from stdin with `version`, `tool` and `input`, and writes one response. Requests are limited to 1 MiB and responses to 16 MiB. Envelope errors, unsupported tools and cancellation before dispatch fail without invoking a tool. A response-write failure may follow a completed mutation and must not trigger automatic replay. The response has an explicit images field because ordinary Harness ToolResult JSON omits image data.

The worker reuses Hand's file, search, todo, project skill and web implementations. Shell execution uses its separate backend/capture path; background processes and external MCP/extensions require separate lifecycle wiring. The worker does not read model configuration, credentials or permission grants. The selected container controls reachable resources and available environment.

Three worker race tests passed, including file roundtrip/rejection, cancellation/short output and byte-exact PNG preservation. Vet and Linux arm64 cross-compilation passed. A native cached-image container test exercised read/write/edit, read-only write refusal and outside-workspace symlink refusal. Its binary hash, exact command records, fixture source and outcome are archived with `tool-worker-integration.json`. The initial edit fixture used incorrect field names; that failure is retained separately.

Parent-side routing, worker packaging, complete credential/network policy wiring, background/MCP/extension execution, native Linux-host qualification and full acceptance remain open. This aggregate patch is staged development work against the unpublished Harness candidate recorded in its manifest, not an integrated release.
