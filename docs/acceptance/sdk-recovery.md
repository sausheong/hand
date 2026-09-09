# Request recovery across SDK sessions

CLI RPC and embedded SDK now use the same workspace-scoped durable ledger helper. Its location is stable when sessions change and remains compatible with the existing CLI RPC path. Embedded construction acquires exclusive ledger ownership before opening or creating sessions. Absolute cleaned workspace paths define identity; symbolic-link aliases are not canonicalised.

A real-runtime SDK regression completes a prompt through a local HTTP provider fixture, switches to a new session, closes and reopens the SDK, then retrieves the original completed terminal by request ID. Reusing that ID for a prompt in the new session returns a conflict and makes no additional provider call. Other tests verify exclusive workspace ownership, uncertain-intent recovery and separate-workspace isolation.

SDK, RPC and CLI uncached race suites and vet pass. Raw results and source hashes are recorded in `sdk-recovery-integration.json`. The earlier unreleased SDK session-scoped ledger files are preserved but not automatically migrated. Full released-package compatibility and final candidate acceptance remain pending.
