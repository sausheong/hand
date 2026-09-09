# RPC session and profile controls

The controller-backed CLI dispatcher now advertises `session.list`, `session.new`, `session.select`, `session.name`, `profile.list` and `profile.select`. Service-only embedders do not advertise unavailable controller methods.

Session listing accepts nonnegative `offset` and `limit` from 1 to 100 (default 50), returning records, next offset, total count and current session ID. Encoded records are bounded to 512 KiB per page. Listing uses a fresh catalogue snapshot; clients should not treat numeric offsets as stable across concurrent catalogue edits.

Session/profile mutations reuse the existing controller reservations and validation. RPC also waits for its prior run consumer to finish terminal persistence before allowing a session mutation. Responses identify the resulting session, profile and model. Profile provider construction uses existing credential configuration without exposing credentials in responses.

The permanent test uses a real session manager/catalogue to create, name and resume sessions while preserving prior history, then switches a local profile without model calls. Its active-run check uses the application service and rejects a new-session request while work is running. Affected RPC/CLI race suites, the strengthened focused test and vet passed; raw evidence is in `rpc-controls-integration.json`.

Only prompt requests currently use durable idempotency. Replaying a session.new request is not yet deduplicated; clients must not automatically retry non-prompt mutations after uncertain delivery. Steering, follow-ups, attachment parity and the complete non-Go client journey remain pending. This does not close M4.1.
