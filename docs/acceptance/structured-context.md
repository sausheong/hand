# Structured task state

Development implementation for M6.2. Structured records are persisted separately from generated summaries and reinjected into generation and summarisation requests. A summary that omits a recorded fact cannot erase it. This complements objective/constraint pins; it does not automatically extract facts from conversation history.

## Record format and limits

Each item has a unique lowercase `id` (letters, digits, underscore and hyphen; 1–64 characters), a `kind`, nonblank `text` (up to 2048 UTF-8 bytes), and an optional opaque `reference` (up to 1024 UTF-8 bytes). Supported kinds are `objective`, `decision`, `unresolved_work` and `verification_reference`. A verification item requires a nonblank reference. NUL and invalid UTF-8 are rejected. A set has at most 32 items and its versioned JSON representation must fit within 8 KiB. The existing separate pin limit remains unchanged; the combined maximum fits the compaction guidance allowance.

References are locators, not executed commands or automatically opened files. Recording a test result does not mark the current workspace verified. The model is instructed to inspect referenced evidence, its scope and tested workspace version before making that claim. A later edit may make referenced evidence stale.

Records are session-wide, including across conversation branch selection. They survive compaction and disk restart. They do not grant permissions or supersede the current user request. A caller explicitly replaces the entire set to update or resolve work. This operation requires the current revision; it cannot silently overwrite newer records.

## Terminal

`/state` lists the revision and current facts. `/state set ID objective|decision|unresolved_work TEXT` adds or replaces one item. `/state evidence ID REFERENCE TEXT` records an evidence locator and its stated scope. `/state remove ID` removes an existing item; a missing ID is an error. Unrelated items are preserved. These commands run as joined idle operations; active runs cannot mutate the set. Revision checks protect the controller read/modify/write operation from concurrent writers.

The real PTY runner `scripts/check_context_state_terminal.py` exercised eight commands and verified five journal revisions and the exact final set. Its raw terminal output and session journal are linked from `context-state-terminal.json`. This is development evidence, with no model prompts or paid calls.

## RPC

`context.state` accepts `{}` and returns `revision` and `items`. A session without records has revision 0. `context.state.replace` requires the read revision and an explicit items array:

```json
{
  "revision": 0,
  "items": [
    {"id": "goal", "kind": "objective", "text": "Deliver the requested migration"},
    {"id": "choice", "kind": "decision", "text": "Retain the old configuration reader during migration"},
    {"id": "pending", "kind": "unresolved_work", "text": "Exercise rollback against a populated workspace"},
    {"id": "tests", "kind": "verification_reference", "text": "Migration unit tests on snapshot abc; rollback not covered", "reference": "evidence/migration/run.json#snapshot-abc"}
  ]
}
```

A successful replacement returns the next revision. Send `items: []` with the current revision to clear. Missing/null items or revision are rejected. Mutations run only while the controller is idle and participate in the durable RPC replay ledger. A replay returns the original response; it does not resurrect an older set. A fresh request with a stale revision is rejected.

Embedding APIs are `Runtime.ContextState`, `Runtime.SetContextState` and their Hand controller equivalents. `context.inspect` includes a `task_state` contribution in its estimated context use. Invalid or unsupported persisted records block generation and compaction instead of being silently ignored.

## Automatic verification references

After `verification.run` saves its evidence file, Hand records the latest evidence reference for that named profile in structured task state. `verification.check` refreshes the assessment and check time, including a stale result after workspace edits. Entries contain the profile/digest, before/after snapshot IDs, exit code and assessment at that time. They never assert continuing validity. Use `/verify-check PROFILE ID` or `verification.check` to check a `hand-verification:ID` locator against current state.

Automatic entries use `verification-` followed by a hash of the profile name. One entry per profile is updated; older evidence remains in the evidence store subject to its retention rules. Unrelated user facts are not evicted. Conflicting non-automatic facts, capacity limits and persistence errors are returned visibly. The evidence file remains available if its context update fails. A failed evidence save does not create a reference. Removing a reference does not delete evidence; deleting evidence does not make a historical reference valid.

Standalone verification controllers without an attached runtime session keep their previous behaviour and do not retain session context. Configured Hand sessions receive the automatic references. Automatic extraction of arbitrary conversation decisions and objectives is still pending; those can be explicitly recorded using `/state` or RPC.

## Compatibility and evidence

The additive annotation kind is `harness.context_state`, with payload version 1. Sessions without this annotation keep their existing behaviour. Older binaries do not inject these records, so downgrading cannot be claimed to preserve the new context behaviour. Keep the session journal and use a supporting version to resume reliance on structured state.

Focused evidence is in `harness-context-state.json` and `context-state-integration.json`: 8 Harness and 2 Hand tests/subtests passed under the race detector, and both scoped vet commands passed. Tests cover both execution paths, repeated compaction/restart, stale writes, explicit clearing, malformed state, summariser delivery, context accounting and RPC replay.

Automatic capture of arbitrary conversation facts, further failure/recovery coverage, broad qualification, released integration and final acceptance remain pending. The current tests prove preservation of explicitly recorded facts, not completeness of automatic fact capture or model compliance with those facts.
