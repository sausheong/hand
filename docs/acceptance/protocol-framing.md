# Protocol framing evidence

The staged `protocol` package adds version-1 request, response and event types without internal Hand or Harness types in their signatures. The JSONL reader bounds encoded input and rejects truncated streams; the request decoder rejects duplicate envelope fields and unsupported versions. The serialized writer stops permanently after a partial frame write to prevent corruption.

Permanent regression tests, the 60-second parser fuzz run and vet output are preserved in `protocol-framing-integration.json`. Fuzz executions are parser inputs, not task or acceptance-scenario counts. The contract is documented in `docs/protocol/v1.md`.

This starts M4.1 and does not complete it: CLI JSONL event publication, RPC dispatch, negotiation, durable idempotency, backpressure, approvals and client journeys remain outstanding. The aggregate patch includes preceding staged Hand work and excludes its temporary go.mod replacement. Its recorded Harness candidate remains unpublished.
