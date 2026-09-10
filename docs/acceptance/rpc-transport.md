# Owned stdio RPC transport

The staged CLI now accepts `--rpc` (incompatible with `-p` and `--jsonl`). It constructs the application without the TUI or stdin workspace-trust questions, opens a private workspace-specific request ledger under the session store, and serves versioned JSONL requests. Initial supported methods are returned by `hello`.

The transport owns its duplex connection and dispatcher. Its connection must unblock reads/writes when closed. It allows one queued request ahead, serialises dispatch responses and enforces a default 30-second response-write deadline. Cancellation closes blocked I/O; EOF or connection failure cancels and joins active application work. The caller closes the ledger afterward. Progress is retrieved through bounded `events.poll`; completed terminal events remain in `request.get` even after progress eviction.

Permanent duplex-pipe tests exercise negotiation and an active-run disconnect, an unread response triggering the write deadline, and cancellation while blocked on input. Affected RPC/CLI race suites and vet passed. A compiled Hand binary also completed a real stdin/stdout hello followed by EOF, emitting one parseable JSON response and exiting zero. That smoke constructed a local provider but made no model call. Raw outputs, binary hash and source hashes are in `rpc-transport-integration.json`.

Approval decisions, steering/follow-ups, session/profile controls, attachment admission, durable progress replay and full non-Go client journeys are not complete. Runtime model calls requiring approval can currently be cancelled through RPC; approval resolution is the next interface control to implement. Native platform and final acceptance remain pending.
