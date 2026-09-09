# Embedded SDK development implementation

`Open(ctx, EmbeddedOptions)` returns a client backed by Hand running inside the caller's process. It uses the same runtime construction helpers, provider adapters, tool registry, session manager, approval broker, background process service and RPC dispatcher as the CLI. Provider construction was extracted to the application package and the CLI now calls that shared implementation.

Options explicitly select the workspace, store, provider/model, endpoint, credential environment reference, session and run limits. The SDK does not load user config. Workspace instructions and skills use existing Hand discovery. Permission state starts empty; mutation approval uses `approval.pending` and `approval.respond`. Workspace permission grants are not implicitly trusted. Custom MCP servers, lifecycle hooks and profile management configuration are not yet exposed.

The context owns the embedded lifetime. Closing the client cancels and joins dispatcher work, closes the ledger, current and initial session leases, runtime connections and background processes. No mutable Harness objects are exported. The API remains experimental. `examples/embedded` lists sessions without a model call.

SDK/CLI uncached race tests pass with local HTTP fixture access; the initial sandbox failure is retained. Tests demonstrate session creation, switching and reopening after cleanup, cancelled construction, and a prompt through the real Harness runtime/provider adapter to a local HTTP fixture with a completed durable terminal result. Vet passes. These tests are development evidence, not native release or live model qualification. Released package examples, compatibility CI and full CLI/RPC/SDK equivalence remain pending.

See `sdk-embedded-integration.json` for the aggregate patch, candidate, source hashes and raw evidence.
