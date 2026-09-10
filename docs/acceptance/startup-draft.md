# Startup draft and retry

Interactive MCP startup now accepts editable prompt text while connections are pending. The draft transfers to the main editor without automatic submission. Both editors use a 65,536-character input limit to avoid truncation during transfer; downstream prompt byte limits still apply when submitting.

Required construction failure remains visible. Ctrl+R retries construction after the previous attempt has joined, retaining the draft; Esc or Ctrl+C cancels and exits after cleanup. Completion messages identify their attempt so an older message cannot settle a newer retry. Ordinary q and r keys enter text. Retry currently rebuilds the whole failed construction; it is not a per-server reconnect operation.

CLI/TUI race suites passed after correcting the initial nonexistent editor-field reference. A subsequent focused test verifies long draft transfer after aligning the input limits, along with startup cancellation, disconnect, success and retry tests. Vet passed. Exact raw results, including the initial build failure, are retained in `startup-draft-integration.json`.

This remains development work against unpublished Harness `daa305d55640a6ca135437b285ae4265da5596a5`. Main local runs while optional connections proceed, per-server reconnect controls and native terminal qualification remain pending. The aggregate patch excludes the temporary go.mod replacement and does not close M3.4.
