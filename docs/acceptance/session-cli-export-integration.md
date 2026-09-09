# Staged offline CLI export

The staged binary supports:

```sh
hand --session SESSION_ID --export-session '/path/to/new export.jsonl'
```

An explicit session ID is required. Export cannot be combined with prompt/provider flags; conflicting invocations fail with exit 2 before work begins. Export success exits 0; operational errors use exit 5 and context cancellation uses exit 130. The destination must be new and its parent directory must exist. The existing 256 MiB total and 10 MiB record limits, 0600 temporary file, exclusive publication and directory sync apply.

This path runs before model configuration, credentials, workspace trust prompts, hooks, MCP or runtime construction. It installs the signal cancellation context before session access. Manager.ExportID validates the catalogue binding and backend identity, acquires the session writer, exports and releases it without changing catalogue selection. Busy or unknown sessions fail; it never creates an empty replacement session for an unknown ID.

A compiled-binary test seeds two durable sessions, leaves the second selected, deliberately writes malformed model configuration and removes provider credentials. A separate process exports the first session to a path containing spaces. The test compares source/export bytes, verifies the active catalogue selection is unchanged and confirms an existing destination is refused. Invocation tests reject missing identity and a conflicting prompt. This is a subprocess CLI journey, not a TUI PTY test or live model evaluation.

Fresh full uncached race/coverage validation passed 541 tests/subtests, zero failures/skips; vet exited 0. Commands ran in `/private/tmp/hand-session-integration-20260908` with GOCACHE=/private/tmp/hand-review-gocache and GOPROXY=off:

```sh
go test -race -count=1 -json -coverprofile=/private/tmp/hand-cli-export-coverage-20260908.out ./...
go vet ./...
```

The aggregate patch and source/raw hashes are in `session-cli-export-integration.patch/json`. Previous snapshots are preserved. The patch excludes the temporary go.mod replacement and passes application checking against primary source. Primary Hand still uses released Harness v0.3.9 and has not received staged session changes; development uses unpublished candidate 5cc93a3. These checks do not close a full requirement group.

Remaining work includes export syscall/process fault qualification, killed-process temporary cleanup, persisted usage and attachment portability, native platform and interactive PTY journeys, released dependency integration and the other M0–M8 obligations.
