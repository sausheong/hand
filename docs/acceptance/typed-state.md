# Typed operational state blocks

Approval requests and decisions now retain their tool name, input, preview and available approval ID as source fields. Compaction updates retain state, summary, affected-turn count and available before/after token counts. Runtime errors and terminal-outcome messages use the typed renderer; untrusted error text is sanitised before display.

Cached presentation is rebuilt from these source fields on width/style changes. Truncated compaction details remain explicitly labelled. Pending approval controls continue to use their existing application ownership and decision protocol; transcript records do not grant approval authority.

The TUI race suite passed 220 test/subtest events; vet passed. New regressions cover approval metadata, compaction state preservation across resize and unsafe terminal sequences in runtime errors. Evidence and aggregate source hashes are in `typed-state-integration.json`.

Administrative transcript messages still have rendered-string paths. Final consolidation into a fully authoritative typed store, asynchronous Markdown resize, delta coalescing and native qualification remain pending. No requirement group is marked complete.
