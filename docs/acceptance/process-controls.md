# Interactive background process controls

This staged change wires the bounded process registry into interactive Hand startup and shutdown. The commands operate on explicit terminal-user input; they are not registered as model tools and grant no permission to subsequent model tool requests. Shell commands execute with Hand's host permissions, without isolation.

Usage:

- `/process start <shell command>` starts work in Hand's startup workspace and returns an invocation-local handle.
- `/process list` reports handles, original commands and running state.
- `/process read <ID>` shows the bounded stdout/stderr snapshots, byte counts, truncation and exit state.
- `/process send <ID> <line>` writes a UTF-8 line with a trailing newline; it is not a raw binary input interface.
- `/process wait <ID>` asynchronously waits while the UI remains available.
- `/process cancel <ID>` requests cancellation; wait reports joined completion.
- `/process forget <ID>` removes a completed record.

Start, send and wait share one owned pending UI operation. List, read and cancel remain usable during it. Foreground goal cancellation does not cancel admitted background work. Exiting Hand cancels and joins background processes and pending process controls. Switching sessions does not restart or relocate these invocation-owned processes. Records are not restored after restart.

The affected app, TUI and CLI packages passed uncached race tests: 370 test/subtest pass events, zero failures and zero skips. Vet passed. Permanent tests cover foreground coexistence, interactive I/O, cancellation while waiting, shutdown joining and late-message rejection. Raw output, hashes and aggregate source patch are recorded in `process-controls-integration.json`.

The aggregate patch excludes the temporary go.mod replacement and depends on unpublished Harness candidate `540dd08a6975ac0076f697668ca861468715cf44`. Primary Hand remains on v0.3.9. Model-facing process tools and their policy admission, MCP startup behaviour, native qualification and final M3.4 acceptance remain outstanding.
