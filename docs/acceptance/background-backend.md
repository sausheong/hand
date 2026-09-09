# Background processes through an execution backend

Harness `execution.Starter` creates owned asynchronous handles for host or container execution. The handle provides input, snapshots, cancellation and waiting while retaining full capture artifacts. Container creation is shared with synchronous execution, so mount, image, environment and network restrictions remain identical. Its cleanup is joined before the handle reports terminal state. Cancellation during input delivery also joins backend cleanup.

`process.StartHandleWithEnv` permits an explicit environment without ambient inheritance. The existing StartHandle entrypoint retains its compatibility behaviour. Host and container execution starters use explicit environments.

Hand `NewProcessesWithBackend` requires a backend supporting these owned handles. Unsupported backends return an error rather than falling back to host processes. Existing TUI and process-tool operations use the same registry interface. The process tool's description reports the configured boundary. Startup still uses the old constructor until configuration integration is completed.

A native container test verifies input delivery, captured output, a denied root write, and joined shutdown of a shell with child work. A cleanup-barrier test verifies that a handle cannot report completion before backend cleanup is done. Full application/TUI race tests and affected Harness execution/process/shell race tests passed with native fixtures enabled; vet passed. Results and raw evidence are indexed in `background-backend-integration.json`.

Startup selection, hooks/MCP/extensions, worker packaging, native Linux-host qualification and final latency/acceptance gates remain open. Container creation still has the separately documented bounded timeout; these tests do not establish final cancellation percentile compliance.
