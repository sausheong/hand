# Typed SDK snapshots

Use `NewPromptRequest(id, text)` and `Client.Submit` for immutable validated prompt requests. `Client.Lookup(callID, requestID)` retrieves an `Execution` snapshot; `Terminal()` returns the persisted terminal when present. An execution state of `completed` describes durable persistence, not necessarily success: inspect the terminal's `Status()`.

`Client.PollEvents(callID, after)` returns an immutable `EventPage`. `Events()` returns a copied slice, and each event's `Payload()` returns copied JSON bytes. Typed getters expose identity, sequence, timestamp, kind, text, status and reason without exposing mutable Harness state. Event kinds remain open for compatible protocol extensions. Terminal request/session/run identity must match its execution record.

`Gap()` reports evicted progress, `Next()` is the next poll cursor, and `Latest()` is the server's latest cursor. Cursors belong to one connection; restart from zero after reconnecting. Retrieve durable terminal records with Lookup when progress has been lost.

Targeted SDK race tests cover real-dispatcher submit/cancel/lookup/poll, copying guarantees, malformed prompt requests and mismatched terminal identity. SDK vet passes. This is development evidence rather than a full suite or released-package compatibility claim. See `sdk-snapshots-integration.json` for source hashes and raw results.

## Invalid and ambiguous responses

The typed SDK rejects duplicate decoded JSON keys and case aliases of known
schema fields in event, execution and event-page objects, including each cursor
item. This prevents a later field from silently replacing a request identity,
terminal outcome or loss indicator. Unknown additive fields and nonterminal event
kinds remain supported; their application-specific schemas are the caller's
responsibility.

`Submit` requires the returned execution ID to match the submitted prompt ID.
`Lookup` similarly binds its returned record to the requested ID. A matching outer
RPC response ID alone does not establish this relationship.

A version-1 terminal requires a nonempty reason, an explicit boolean `verified`,
and one of `completed`, `cancelled`, `verification_failed`, `budget_exhausted` or
`infrastructure_error`. Only `completed` may carry `verified: true`. Records
missing the verification field are rejected, matching the current server
contract. Verification refers to configured validators, not general correctness.
The flag remains available through the copied `Payload()`.

Event pages require `events`, `next`, `latest` and `gap`. Cursor and gap scalars
cannot be null. Both an empty array and the server's null event-list encoding are
accepted. Existing page-size, cursor-ordering and terminal identity checks still
apply.

Treat a snapshot decoding error as an unusable response. Do not infer success or
absence of lost events from it, and do not resubmit a possibly executed operation
under a new request ID. Reconnect to a compatible server and reconcile using the
original ID. If the response remains invalid, retain the error and report the
protocol incompatibility rather than inventing an execution outcome.

Permanent regression evidence is recorded in `sdk-event-integrity.json`,
`sdk-execution-integrity.json`, `sdk-page-integrity.json`, `sdk-page-fields.json`,
`sdk-submit-identity.json` and `sdk-terminal-outcome.json`. These development
checks do not replace final candidate and released-package qualification.
