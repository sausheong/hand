# Steering claim reconciliation

After the Harness stream and its producers join, Hand verifies claimed corrections against the bound session. A successful flush is required before interpreting history: confirmed delivered IDs are removed; absent IDs return to pending/editable state. Conflicting payloads, session mismatch and persistence errors retain claims unchanged. All claims are validated before any queue mutation.

Reconciliation tests passed 20 race-enabled repetitions (140 test/subtest events), covering unwritten and written-without-acknowledgement corrections after session reopen, failed writer, wrong session and conflicting identity. These tests preserve the queue owner in memory and do not constitute process-kill or durable queue restart evidence. The full Hand race suite passed 610 tests/subtests without failures/skips; vet passed.

See [usage](queued-input-usage.md), [aggregate patch](steering-reconciliation-integration.patch), and [hashed evidence](steering-reconciliation-integration.json).

Uncertain writer failures remain explicit and unresolved. Durable queue restart, attachments, external editor, performance/platform qualification and full acceptance remain pending. Integration is staged against unpublished Harness; primary Hand remains on v0.3.9.
