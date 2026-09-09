# Cross-interface cancellation compatibility

The external-module compatibility fixture now executes normal completion and active cancellation through CLI JSONL, subprocess SDK and embedded SDK. Cancellation starts only after streamed text proves the provider request is active. The CLI receives SIGINT; SDK clients send cancel while keeping their connection open.

Every cancelled run must deliver exactly one cancelled terminal, and SDK event terminals must agree with durable request records. The CLI must exit 130. The local HTTP fixture independently observes request context cancellation for all three interrupted runs. The successful completion journeys remain required, bringing the total to six provider fixture calls, with three cancellations.

The development external-module run passed all six observations; vet passed. It used no live model calls. This is lifecycle development evidence, not the repeated native cancellation latency gate or released-package acceptance. Approval, non-Go clients and the remaining full interface contract still require acceptance evidence. The runner command is documented in `sdk-compat.md`; its final report now requires six calls and three provider cancellations.

Evidence: `sdk-cancel-integration.json`.
