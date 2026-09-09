# Embedded SDK execution selection

`EmbeddedOptions.Execution` accepts `sdk.ExecutionOptions`, selecting the same explicit host/container boundary as the CLI. Container settings include absolute Docker/socket/worker paths, immutable image ID, required worker SHA-256, writable workspace and network flags. User configuration is not loaded implicitly.

Container mode also requires `AuthorityDirectory` outside the workspace. The SDK includes execution options in its scoped authority fingerprint. Reopening an authority directory under an incompatible fingerprint fails explicitly; it does not reuse grants for changed capabilities. Existing model/profile switching safeguards continue to reject changes without an authority rebinding factory.

Startup probes the shell and pinned worker, then routes built-in tools and background processes through the shared backend. RPC hello reports the effective execution boundary. Model instructions explain that shells run in `/workspace` and should use workspace-relative paths. Model-provider requests remain on the host. Custom MCP/hooks remain outside the currently exposed embedded options.

The external-package SDK test suite passed with native fixtures enabled. The real scoped journey approved a write persistently, reused its grant in a second session, revoked it, denied a third write and verified that content remained unchanged. Additional tests reject missing external authority and an incorrect worker digest. Vet passed. Raw evidence is in `sdk-isolation-integration.json`.

This is development integration against the unpublished Harness candidate. Release packaging, richer credential routing, native Linux-host qualification, final coverage/performance/live evaluation and full acceptance remain open.
