# Public Go RPC client development slice

The `sdk` package provides `NewClient`, `Hello`, `Prompt`, `Cancel` and general `Call`. It owns a duplex connection whose Close must unblock reading and writing. Concurrent calls are serialized. Each call uses an explicit request ID; responses must match both the ID and protocol version. Results are independently owned JSON values. No mutable Harness runtime is exposed.

Cancellation before admission leaves the transport intact. Cancellation during I/O closes the connection and execution may be uncertain. Reconnect and query the original execution request ID before deciding what to do next. Non-execution mutations still lack durable deduplication; do not blindly retry them. Close is concurrent-safe and idempotent.

`examples/rpc` starts an installed Hand binary, negotiates capabilities and prints the response without making a model request. The API is experimental pending released-package acceptance. Four SDK tests cover real dispatcher prompt/cancel/terminal lifecycle, blocked-read cancellation, concurrent calls and mismatched response rejection. Vet passes. An external module builds against a local replacement; this is development evidence only, not the released-package scenario.

The embedded API, full interface equivalence, compatibility CI and released package examples remain required. Aggregate implementation and raw development evidence are recorded in `sdk-rpc-integration.json`.
