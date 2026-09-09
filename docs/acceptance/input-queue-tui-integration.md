# Terminal follow-up queues

The staged terminal now exposes `/followup` and `/queue` list/edit/remove/run commands. Follow-ups dispatch automatically after successful goal completion and joined completion checks; cancellation or failure leaves remaining input pending for explicit dispatch. Multiline text and stable queue IDs survive edits. Slash command input during approval cannot accidentally answer the approval, and queue commands leave that approval intact. Terminal queue text is sanitised before display.

Tests cover multiline edit/remove and automatic delivery, approval collision, and cancelled-run retention followed by explicit execution. They passed 20 race-enabled repetitions (60 events). The full race suite passed 600 tests/subtests with no failures/skips, and vet passed.

See [usage](queued-input-usage.md), [aggregate patch](input-queue-tui-integration.patch), and [hashed raw evidence](input-queue-tui-integration.json).

Tool-boundary steering, queued attachment processing, durable delivery/retry reconciliation and full M3.2 acceptance remain pending. The queue is process-local; users must not assume pending entries survive application exit. Primary Hand remains on Harness v0.3.9; this is staged integration with the unpublished candidate.
