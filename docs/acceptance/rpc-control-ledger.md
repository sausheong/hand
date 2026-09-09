# Durable control-operation responses

RPC session creation/selection/naming, profile selection, steering/follow-up enqueue, queue edit/remove, approval response, cancel and follow-up resume now persist intent and response in the workspace ledger. Request IDs are unique operation identities: reusing one with a different method or payload conflicts. Retrying an identical completed operation returns its stored response, including after reconnecting or changing sessions. This does not repeat the mutation.

Records for these methods use `kind: control` and an `operation_id`; run records keep their existing format. A control record's result is the original protocol response, whereas a run record's result is its terminal event. SDK Lookup is explicitly for run records; general request.get can inspect both. Pending/accepted control records reopen as uncertain and require reconciliation, never automatic replay.

The per-record limit now accommodates a maximum protocol frame plus 4096 bytes of ledger metadata; the 64 MiB ledger and 1024 request limits remain enforced. This allows accepted 64 KiB queue text, including JSON escaping, to have its response persisted. Failed operations also retain their original response; a deliberate new attempt requires a new request ID.

Tests cover duplicate enqueue without a second queue entry, replay across restart/session changes, uncertain controls, large response persistence and repeated session.new after SDK reconnect. RPC and SDK/CLI race suites and vet pass. The initial approval fixture failures are retained; those fixtures previously reused one ID for different payloads, now correctly treated as conflicts. Native crash and final released-candidate acceptance remain pending.
