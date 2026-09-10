# Bounded text presentation batches

The production application stream now groups immediately available text deltas into one UI update and viewport refresh. Each original event is still applied in order, preserving its model/state metadata. The collector never waits for another delta. It stops after at most 32 events, once collected text reaches the application event text limit, or immediately after including the first non-text boundary.

Including that boundary in the same update makes approvals, tool events and completion state visible without waiting for a separate text-refresh cycle. No events after the boundary are collected. A final event can take the batch's text total up to twice the single-event text limit; event payloads remain independently bounded by the application service.

The TUI race suite passed 228 test/subtest events; vet passed. Regressions cover exact concatenated text, 32-event limits, full-size text events, approval ordering and no wait for a subsequent delta. Evidence is in `presentation-batch-integration.json`.

The legacy Runtime forwarding adapter still sends individual events. Native latency qualification and full M3.3 acceptance remain pending. This batches presentation work; it does not alter or coalesce durable application event identities.
