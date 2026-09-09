# RPC approval decisions

Negotiation now advertises `approval.pending` and `approval.respond`. Pending returns at most 16 approval events and 512 KiB of event data, plus a remaining count. Resolving returned approvals permits subsequent pending calls to retrieve the others. These capabilities remain available independently of progress-ring eviction and are bounded by the application's pending-approval limit.

Respond requires `run_id`, `approval_id` and decision `once`, `deny` or `always`. Run identity must match the published event; the opaque approval ID must still be pending. The dispatcher passes the full current application identity to Service.RespondApproval, which enforces generation, cancellation and single-answer checks. Tool names or preview text cannot substitute for an approval capability. Accepted responses immediately remove the pending entry. Run completion removes any remaining entries, including when terminal persistence fails.

`always` retains the existing approval bridge's persistent-grant semantics and warnings; RPC does not invent broader grants. The permanent tests here exercise once, denial and cancellation with a controlled backend using the real application approval hook. Wrong-run and invalid decisions are rejected. This is not yet the final provider/tool/non-Go client approval journey.

RPC package race-suite and vet results are recorded in `rpc-approval-integration.json`. Steering, follow-ups, session/profile methods, full live-independent client journeys and final acceptance remain pending.
