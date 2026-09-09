# Owned asynchronous process handles

Harness now exposes StartHandle, Snapshot, Send, Cancel, Wait and Close over its existing process-group backend. A handle belongs to its parent context and does not survive owner shutdown. Wait joins the process, descendants, input worker and capture finalisation before returning.

Each stream retains a 64 KiB in-memory prefix and uses the bounded artifact store for larger captures. Input messages are limited to 64 KiB with one queued write. Send reports successful pipe delivery; cancellation after admission stops and joins the owned process because a partial write cannot be withdrawn safely. This behaviour is explicit, rather than silently delivering cancelled input later.

Tests exercise input/output exchange, a verified 70,000-byte capture, owner cancellation, oversized input and cancellation of a blocked input write. The process race suite and vet passed. The original handle tests also passed 20 repetitions; the blocked-input regression was added afterward and passed in the final process suite. See `harness-process-handles.json` for exact evidence.

This starts M3.4. Hand's bounded handle registry, UI/tool controls, policy integration and optional/required MCP startup are still pending. Native child-process qualification and full candidate regression suites remain required. Process groups provide lifecycle management, not a security sandbox.
