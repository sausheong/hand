# RPC steering and follow-up queues

Negotiation now advertises `steer`, `followup.enqueue`, `followup.start`, `queue.list`, `queue.edit` and `queue.remove`. Steering and follow-up admission use the existing session-scoped application queues and their input limits. Editing/removal reuse claimed-input protections. Listing accepts an offset and returns at most 16 records and 512 KiB of encoded data.

`followup.start` starts the oldest follow-up through Service.StartFollowup, including its attachment resolver and atomic admission checks. The start request uses the durable ledger: replaying its ID returns the prior run and cannot consume the next queued message. Queue creation/edit/removal themselves are not yet durably deduplicated. Automatic RPC follow-up continuation remains unfinished; this is the explicit-start foundation.

The tests exercise queue selection, editing/removal, exact oldest-follow-up delivery and duplicate start without consuming a second entry. Separate tests verify that parent cancellation and input disconnect interrupt blocked follow-up resolution. The reader now cancels application ownership immediately on EOF/error rather than waiting for a blocked dispatch call to return. Closing the dispatcher also cancels before acquiring its dispatch mutex.

RPC package race-suite and vet evidence is in `rpc-queue-integration.json`. These adapter tests do not establish model steering at a real provider/tool boundary or complete the non-Go client journey. Full CLI regression, automatic follow-up behaviour and final acceptance remain pending.
