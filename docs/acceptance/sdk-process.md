# SDK subprocess ownership

`StartProcess(context.Context, ProcessOptions)` launches an explicitly configured executable without a shell. Options select arguments (normally including `--rpc`), directory, environment, stderr writer and shutdown timeout. Nil environment inherits the parent; an explicit environment is copied. The context covers the subprocess lifetime.

The returned client owns both pipes and the child. Close signals EOF and releases blocked I/O, then waits up to the configured timeout (default five seconds) for normal cleanup. On timeout it kills and reaps the process and returns its exit error. Close is idempotent. A custom stderr writer must not block indefinitely.

Tests verify cleanup before a cooperative child exits and bounded termination of an uncooperative child. The SDK race suite passed six behavioral tests plus its subprocess helper entry point, with no skips. Vet and the Hand build passed. A separate external Go module using a local Hand replacement launched the freshly built Hand binary with `--rpc --model local/test`, negotiated capabilities, then closed successfully. It used an isolated HOME/workspace and made no model calls.

This development smoke is not released-package qualification. The embedded SDK and full interface compatibility acceptance remain outstanding. Source hashes, aggregate patch and raw evidence are in `sdk-process-integration.json`.
