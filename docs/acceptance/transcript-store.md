# Authoritative typed transcript store

Every production transcript write now goes through typed source APIs. User/assistant messages, tools, approvals, compaction and notices retain source separately from their rendered projection. Clear, banner insertion and session replacement use the same API. A stale or modified render-cache entry cannot overwrite source data. Administrative replay strings are sanitised and stored as notice text.

An AST regression rejects production assignments to the transcript projection outside its owning module. Additional tests verify projection recovery and clearing. The TUI race suite passed 222 test/subtest events; vet passed.

The updated benchmark uses 10,000 typed notice blocks at 80×24. Across 30 runs and 1,020 measured events on Apple M4 Max/macOS 26.6.2/Go 1.25.1, p95 was **0.237958 ms** and maximum **0.291625 ms**. Each event includes a stream delta, key handling and View construction. Initial layout, fixture creation and physical terminal paint are excluded. One-event Go calibration samples remain in raw output but are excluded from these metrics.

Source hashes, raw test output and all timing samples are preserved in `transcript-store-integration.json` and its referenced evidence files. Markdown-heavy resize still runs synchronously. Delta coalescing, native qualification and complete M3.3 acceptance remain pending.
