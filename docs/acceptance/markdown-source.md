# Assistant source retention

Completed assistant messages now retain typed Markdown source independently of rendered presentation. Live stream completion, initial history replay and session switching populate the source records. Width or style changes rebuild rendered Markdown from source rather than wrapping an already-wrapped result. Banner insertion shifts source indices; clearing or replacing history discards obsolete sources.

Terminal sanitisation runs before every source render. Replayed raw source can therefore be preserved without allowing its control sequences into the terminal. The rendered cache records width and style, and unchanged blocks avoid repeated Markdown work.

The TUI race suite passed 212 test/subtest events, including source-preserving resize, returning to the original width, banner shifts, replay sanitisation and stale-source rejection. Vet passed. Source hashes, raw evidence and the aggregate integration patch are recorded in `markdown-source-integration.json`.

This is a partial typed-transcript migration. Other block kinds remain to be migrated. Resize currently rerenders retained assistant Markdown synchronously; the earlier 10,000-block plain-text benchmark does not qualify Markdown-heavy resize. Delta coalescing, native performance and full M3.3 acceptance remain pending.
