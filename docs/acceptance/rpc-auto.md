# Automatic RPC follow-ups

`followup.enqueue` accepts `{"text":"next task","auto":true}`. Its response includes the queued input and `request_id` for the resulting execution. Automatic execution begins immediately if idle, otherwise after the preceding successful run has persisted its terminal result. The oldest follow-up determines ordering; a manual entry prevents automatic entries behind it from overtaking.

Cancellation, unsuccessful outcomes and persistence or admission failures pause automatic execution. `state` exposes `followup_paused`; `followup.resume` explicitly resumes while idle. Enqueuing more work does not unpause a cancelled queue. Use `followup.start` for manual entries. Repeating a completed start request retrieves its durable record even when an automatic entry is now at the queue head.

Automatic execution uses a durable request ID derived from its queue ID. Previously recorded execution intent is never automatically repeated; uncertain admission requires reconciliation. Queue contents and automatic intent remain connection-local. Enqueue and other non-execution mutations are not yet durably deduplicated, so clients must not blindly retry them.

Evidence in `rpc-auto-integration.json` is development RPC race testing, including FIFO execution, durable predecessor terminals, cancellation/resume and replay regression tests. It is not full milestone or release qualification. The aggregate patch requires the development Harness candidate recorded in its manifest; primary Hand remains on released Harness v0.3.9.
