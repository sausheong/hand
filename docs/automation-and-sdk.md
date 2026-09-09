# Automation and the Go SDK

The CLI, RPC server and embedded SDK share Hand's application operations. The SDK
and protocol are development interfaces until released-package compatibility
qualification passes. Pin the exact Hand version and required Harness dependency
for an integration; do not assume that an older published package contains the
interfaces in this checkout.

## Choose an interface

Use `hand -p "request"` for one bounded command and its exit status. Add `--jsonl`
for machine-readable progress and a terminal event on stdout. Diagnostics go to
stderr. Admission failures can exit before a terminal event exists. One-shot mode
cannot receive approval responses; gated operations are denied unless explicitly
authorised by the invocation or existing scoped policy. `--yes` grants broad
approval for that invocation and should be an intentional caller choice.

Use `hand --rpc` for a long-lived client that manages sessions, approvals,
cancellation and recovery. Stdout is reserved for newline-delimited JSON; keep
logs on stderr. Negotiate with `hello` before other methods and inspect advertised
capabilities. See the [version 1 protocol](protocol/v1.md) for framing and methods.

Use the Go package `github.com/sausheong/hand/sdk` either to own a Hand subprocess
or to embed its application service. Both return the same `*sdk.Client` contract.

## Start with the runnable examples

From this checkout:

```sh
go run ./examples/rpc /absolute/path/to/hand
go run ./examples/embedded /absolute/workspace /absolute/private/session-store
```

The RPC example starts Hand with `--rpc` and prints negotiated capabilities. It
inherits the environment and uses the current working directory, so configure
Hand's model/endpoint for that workspace first. Startup can initialise session
and permission stores and configured integrations. The example does not submit
a model prompt. The embedded example selects `local/test`, opens a session store,
negotiates and lists sessions without a model call. Use disposable directories
when exploring either example.

Read the complete [subprocess example](../examples/rpc/main.go) and
[embedded example](../examples/embedded/main.go). `sdk.StartProcess` takes
`ProcessOptions`; include `--rpc` in `Arguments`, set `Directory` to the intended
workspace, and supply an explicit `Environment` when inheritance is unsuitable.
A nil environment inherits the parent. `Stderr` must accept writes promptly.

`sdk.Open` takes `EmbeddedOptions`, including workspace, store directory, model,
endpoint and optional authority/checkpoint/execution settings. Set
`AuthorityDirectory` to opt into private scoped permission storage outside the
workspace; an empty value retains the migration-era approval mode. Embedding
alone does not enable container isolation. Inspect the effective boundary and
configure the execution options explicitly when required.

## Track one request through completion

1. Call `Hello` with a fresh negotiation ID.
2. Construct `sdk.NewPromptRequest(requestID, text)` and call `Submit`. Keep that
   immutable request and its ID until the outcome is reconciled. Submission
   acknowledges execution; it does not mean the requested work has succeeded.
3. Call `PollEvents` with fresh call IDs and the last page's `Next()` cursor.
   Process the returned snapshots and check `Gap()`. Lost progress is explicit;
   retrieve the durable outcome with `Lookup(callID, requestID)`.
4. For approvals, use `Call` with `approval.pending` and `approval.respond`.
   Copy the current run and approval IDs. Present the actual operation to the
   user before choosing `once`, `deny` or the persistent `always` decision.
5. Inspect the execution's terminal event and status. A completed answer is not
   automatically verified coding success; inspect its verification field and
   the checks/evidence supporting it.

Each control mutation also needs a stable request ID. Reusing an ID with changed
parameters conflicts. A transport failure leaves execution uncertain: reconnect,
select the same session and query the original request ID. Do not retry a
possible side effect under a new ID merely because its response was lost.
An uncertain retained record requires reconciliation, not automatic replay.

Event cursors belong to one connection; restart at zero after reconnecting.
Read-only calls such as polling still need response-correlation IDs. The client
serialises calls, and returned typed snapshots do not expose mutable internal
state. Use `Call` for advertised methods without a typed wrapper.

## Cancel and close deliberately

`Client.Cancel` requests cancellation through RPC while preserving the connection
for outcome inspection. Cancelling an in-flight call's context closes the
connection because execution might already have started. The context passed to
`StartProcess` or `Open` owns the entire application lifetime, not just startup.

Always close the client. Subprocess close signals EOF, waits for cleanup, then
kills and reaps the owned process if its shutdown timeout expires (five seconds
by default). Embedded close cancels and joins owned work and session resources.
Inspect close errors when cleanup is part of your success condition. A new client
must not overlap a still-owned session writer.

## Interpret outcomes and evidence

One-shot exit codes are 0 for completed, 2 for invalid invocation/configuration,
3 for validation failure, 4 for exhausted limits, 5 for infrastructure/provider
failure and 130 for cancellation. Keep stderr and exit status alongside JSONL
output. Unknown token usage is not zero usage; check the protocol's usage-certainty
fields before treating totals as complete.

For durable file and verification operations, follow
[sessions and restore](sessions-and-restore.md). For grant migration and
revocation, follow [permission migration](permission-migration.md). Real provider
calls incur the provider's charges; the negotiation/session-list examples above
do not establish model quality or superiority to Pi.

### Signals during startup

In one-shot and RPC modes, SIGINT and SIGTERM also cancel startup, including pending MCP connection handshakes. Hand joins owned startup children before exiting with code 130. A cancellation before an application run starts produces no run terminal event; capture stderr and the process exit code as startup diagnostics.

RPC also observes client input EOF during initialization and cancels pending startup work. The same bounded reader supplies queued input to the dispatcher once construction completes. A normal negotiated connection still closes on EOF; an EOF during cancelled startup may exit with code 130. Linux/macOS RPC uses owned interruptible stdio descriptors so pending reads and writes can be joined on shutdown.
