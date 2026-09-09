# Background optional MCP discovery

Interactive Hand constructs required MCP servers first, then opens its main editor while optional servers connect independently. Without required servers, local work is immediately available. One-shot mode retains bounded synchronous discovery. Names are validated across both server groups before partitioning.

The application manager permits up to 32 optional configurations and four concurrent connections, each with a five-second default deadline. A discovered client remains `ready` while the application is busy. Attachment reserves an idle application operation, updates runtime tools and prompt hints, and transfers ownership to Runtime. Consequently the model does not acquire new tools midway through an application goal. Failed/untransferred clients remain the manager's responsibility and are joined on shutdown. Transferred clients close with Runtime.

`/mcp` refreshes status: queued, connecting, ready, connected or unavailable. `/mcp retry <server>` retries an unavailable optional server; it cannot duplicate a queued or connected attempt. Startup shows initial status and the refresh command. Required-server failures retain the startup screen's whole-construction retry. No automatic reconnection policy or process survival across restart is implied.

Real stdio regression tests hold the application busy during discovery, verify no premature tool publication, then release it and verify the model catalogue and ownership transfer. Further tests cover retry and shutdown. Affected app/TUI/CLI race-suite output and hashes are in `mcp-background-integration.json`; vet passed. Native editor responsiveness, all end-to-end journeys and full exact-candidate acceptance remain pending.

The aggregate Hand patch excludes the temporary go.mod replacement and requires unpublished Harness `328ce04dad7ff160217cdb4894c377f124a5e1d8`. Primary Hand remains on v0.3.9, and M3.4 remains in progress.
