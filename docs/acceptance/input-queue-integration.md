# Application-owned input queue foundation

Added separate typed steering/follow-up queues bound to the current session, with immutable snapshots, stable IDs, editing and cancellation. A shared limit of 64 entries and 64 KiB UTF-8 text per entry bounds retained input. `StartFollowup` atomically takes the oldest follow-up and claims run ownership only after the previous backend, completion checks and terminal persistence have joined. Failed admission leaves the queue unchanged. Accepted input becomes a run and is not automatically requeued, avoiding silent duplicate execution.

Queue checks passed 20 race-enabled repetitions (60 tests). The full suite passed 597 tests/subtests without failures/skips; vet passed. A held completion-check test verifies admission timing and edited input delivery; tests also cover snapshot isolation, cancelled admission, absent backend, invalid input and limits. An initial sandbox listener failure is retained in the evidence.

See [aggregate patch](input-queue-integration.patch) and [hashed evidence](input-queue-integration.json).

This is an application API foundation, not completion of M3.2. TUI controls, automatic follow-up dispatch, tool-boundary steering, attachment input and durable delivery/retry reconciliation remain outstanding. Primary Hand still uses Harness v0.3.9; staged integration uses the unpublished local candidate.
