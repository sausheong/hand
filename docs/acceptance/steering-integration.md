# Hand steering integration

HarnessBackend now connects the application-owned steering queue to the Harness tool boundary. The oldest correction is claimed with immutable text and a stable ID; acknowledgement removes it only after Harness confirms persistence. Repeated acknowledgement is harmless. Claimed entries cannot be edited or removed while delivery is unresolved.

The terminal exposes `/steer`, and `/queue` identifies claimed entries. Steering command submission during an approval does not answer that approval. An integrated provider/tool test holds the first tool, queues a correction, then verifies the second tool does not run and the next model request contains the correction.

The integration/claim tests passed 20 race-enabled repetitions (40 events), and terminal submission passed 20 repetitions. Full Hand race validation passed 603 tests/subtests without failures/skips; vet passed. See [usage](queued-input-usage.md), [aggregate patch](steering-integration.patch), and [hashed evidence](steering-integration.json).

Delivery-failure reconciliation, process restart, queued attachments and full acceptance remain pending. Hand uses serial tool boundaries while this source is installed; speculative streaming kickoff is disabled. This remains staged against unpublished Harness; primary Hand uses v0.3.9.
