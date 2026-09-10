# One-shot JSONL events

The staged CLI accepts `hand --jsonl -p 'prompt'`. Application progress is encoded into version-1 event envelopes and followed by one terminal event after the run joins. Default text output remains unchanged. JSONL uses invocation-unique event IDs and preserves application sequence, timestamps and session/run identities. The one-shot request identifier is `oneshot`; event IDs are not durable replay IDs until retained by the future request ledger.

Payloads contain explicit snake-case fields for state, text, outcome, tool calls/results, usage and compaction. Tool input/output/metadata are bounded display snapshots; `truncated` discloses truncation. This is not full artifact retrieval. Output write failure cancels and joins the run before returning infrastructure failure. A writer that blocks without returning still needs transport-owner cancellation, which remains part of RPC/backpressure work.

JSONL mode never reads stdin for workspace-trust confirmation. Existing trusted grants retain their meaning; untrusted project grants stay inactive. Tool mutation requests therefore follow the one-shot unresolved-approval outcome unless explicitly authorised by existing options. Diagnostics remain on stderr. Errors before run acceptance may produce no event and a nonzero exit; they do not fabricate an accepted run.

Permanent tests exercise ordered event decoding, embedded newlines, exactly one terminal result, output-failure joining and an input reader that panics if hidden trust input is attempted. Affected CLI/application race-suite evidence and vet output are in `jsonl-events-integration.json`. Full CLI subprocess journeys, bidirectional approval decisions, retention/replay and complete M4 acceptance remain pending.
