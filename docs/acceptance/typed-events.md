# Typed event source blocks

User, assistant, tool-call and tool-result rendering now retains source separately from cached presentation. Session replay also preserves compaction summaries/counts and notes. Tool inputs remain immutable strings; result output and errors are retained independently. Live runtime events, application events and replay share the source renderer.

This increment corrects a missing integration in the earlier output viewer: production application events rendered previews but did not populate the retained output list. They now do. Permanent tests exercise this application path, compare it with replay and verify that the final captured lines are reachable in the viewer.

Application events carry bounded display details. When those details are truncated, the preview and viewer say so explicitly; this does not constitute retrieval of the complete underlying session record or captured artifact. Full artifact access remains pending.

The TUI race suite passed 214 test/subtest events and vet passed. Evidence and aggregate sources are in `typed-events-integration.json`. Approval/status paths, live compaction source migration, asynchronous Markdown resize and delta coalescing remain incomplete. No M3.3 acceptance group is closed.
