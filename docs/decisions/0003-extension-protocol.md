# Extension host protocol

Status: implementation started; activation, package trust and host integration remain pending.

M7 requires persistent subprocess extensions, independently of client RPC and MCP. The public Go package `extension/protocol` defines the initial newline-delimited JSON framing layer. This does not enable discovery or execution of project-local code.

Every frame has version 1, kind (`request` or `response`) and an ID of 1–64 ASCII letters, digits, periods, underscores or hyphens. Requests contain a method using the same identifier syntax and an object-valued `params`. Responses carry exactly one `result` (including explicit JSON null) or structured `error`. Frames are at most 256 KiB excluding their required newline. Unknown envelope fields, unsupported versions, duplicate JSON keys at any depth, nesting beyond 32 and invalid UTF-8 are rejected. A failed reader stays failed so an oversized or malformed message cannot be followed by bytes reinterpreted as a new request. Concurrent outgoing frames are serialised; short writes are errors.

This transport is deliberately separate from authorisation. The future host must validate method-specific schemas, correlate each response with an outstanding request, apply capability policy before servicing callbacks, and terminate failed peers. Generic method syntax is not a method allowlist or permission grant. Error text is data and must pass through terminal sanitisation. Extensions must render declarative blocks, never write arbitrary escape sequences to the terminal.

The host will explicitly approve executable identity and capabilities before startup. Persistent processes need bounded stderr, cancellation, joining and health state. Context transformations need stable ordering and a protected separation between mutable context data and user policy. Mandatory policy-hook failures must deny work. Transactional reload must stage and validate a changed process before replacing the previous healthy one at an idle boundary. An arbitrary host subprocess retains OS permissions unless an enforced isolation backend is used; declarations are not a sandbox.

Outstanding implementation: host/session lifecycle, method schemas and capability dispatch, declarative presentation and user questions, extension state persistence, ordered transformations and policy hooks, transactional reload, Go/Python examples, package installation/integrity and user controls. No extension acceptance scenario is complete from framing tests alone.

## Connection ownership implementation

`internal/extensions.Connection` now owns an explicitly supplied transport with input/output/stderr pipes, a stop callback and a cleanup-joining wait callback. It never discovers or starts a process. A product launcher must first approve the executable/boundary and ensure stopping the transport kills its owned process tree and unblocks its pipes.

Host requests are serialised, carry generated IDs and default to a 30-second maximum lifetime (a shorter caller deadline wins). A cancelled queued request does not interrupt the active call. Cancellation after admission terminates the connection, joins the blocked writer and waits for process/reader cleanup; no automatic replay assumes the request had no effects. Wrong IDs, peer requests without a callback dispatcher, malformed frames, EOF and process exit close the peer. An ordinary structured remote error remains a call refusal and permits subsequent calls. Diagnostics retain at most 64 KiB; more than 1 MiB stderr traffic terminates the peer. Diagnostic text is data, not terminal output.

Real test subprocesses exercise repeated calls, remote refusal, crash, wrong IDs, timeout, blocked stdin, unsupported callbacks, excess diagnostics and queued cancellation. This validates transport ownership; it does not qualify package trust, host-mediated capabilities or the future extension method contracts.

## Handshake, presentation and questions

The typed initialize response declares version, admitted name, capabilities, commands and lifecycle subscriptions. `Connection.Initialize` sends a snapshot of the host-approved capabilities and checks the response identity, strict schema and subset relationship before returning any registrations. Invalid peer responses close and join the connection. Unknown capabilities and duplicate registrations are rejected. This validates a prior approval; it does not create one. Product activation must namespace command names and apply host policy independently of declarations.

Presentation consists of up to 32 text, code or list blocks, with a 64 KiB total text limit and smaller per-field limits. There are no arbitrary styling, permission, tool-result or terminal-outcome fields. UTF-8 control characters (except tab/newline) and format controls are rejected, including ANSI escape, C1 and bidi controls. The eventual host renderer must label extension provenance and must not feed content into an escape-interpreting renderer.

Questions have a stable ID, bounded title and up to 12 uniquely identified options, with optional free text. Responses must identify the question and contain exactly an offered choice, permitted free text or explicit cancellation. Cancellation cannot carry an answer. The host still needs pending-question ownership, UI/RPC delivery and cancellation integration; schema validation alone does not provide those behaviours.

## Callback transport

A validated connection can now dispatch extension requests made during an outstanding host call. `SetCallbackHandler` installs trusted host code only while idle; capability grants remain the declared subset validated at the one-time handshake. Callback methods map to specific capabilities: user.question, state.get/state.set, file.read/file.write, network.fetch and process.run. Unsupported/unapproved callbacks receive a denial without reaching the handler. Callback declarations do not bypass resource policy; concrete Hand handlers must validate method inputs and apply normal file/network/process rules.

Callbacks are limited to 32 per host call. IDs must be unique within that call and distinct from its host request ID. Unsolicited, duplicate or excessive callbacks terminate the peer. Each handler receives cancellation tied to both the owning request and connection. Call completion after cancellation waits for the handler and process cleanup. A handler must respect that context, join its work and avoid recursive calls to the same connection. Structured host refusals propagate as errors; transport code does not convert them into success. Question delivery, persistent state adapters and concrete resource handlers remain pending.

## Persistent extension state handler

`StateStore` binds a selected session and a host-supplied stable package identity. Its annotation key hashes that identity; callback parameters cannot select another package or session. The package identity must come from trusted activation metadata, not the peer's self-reported name. `state.get` takes `{}` and returns `{revision,data}`. `state.set` requires the current revision and an object-valued data payload; use `{}` to clear. Atomic annotation revision checks prevent concurrent owners from silently overwriting each other.

State is limited to 16 KiB, validated both before and after JSON encoding, with at most 256 retained revisions per package/session. Reaching the revision quota fails visibly and preserves the last state; no automatic journal rewriting or quota-reset migration exists yet. The raw data budget is therefore at most 4 MiB per package/session plus bounded envelopes. Product activation must also bound the number of active packages. Unsupported/corrupt records block reads and replacements. This versioned additive annotation survives session compaction and restart, but an older binary does not provide the handler.

The handler services only state methods. Resource requests sent to it are denied. Host callback transport still checks the negotiated state capability before dispatch. Real subprocess tests write state through the callback and recover the exact value after closing/reopening both the peer and the session. Product session switching must join old peers before closing their bound session and create new handlers for the selected session.

## Ordered context and policy pipelines

`NewHooks` snapshots at most 16 host-configured hooks with ordering and mandatory settings, checking negotiated capabilities. Ordering is ascending priority, then stable ID. Peers cannot change this ordering or mark themselves optional. Context contributions are limited to 64 items/64 KiB, with 16 KiB per item. Only retrieval and tool-result contributions are sent for transformation. User, system and policy contributions remain private to the host and unchanged. Results contain an explicit replacement array keyed by existing editable IDs. New, duplicate or protected IDs reject the entire hook's replacement batch. Optional failures preserve that batch's input and produce diagnostics; mandatory failures stop the pipeline. Original caller data is copied.

Policy hooks can restrict only a host-authorised action. Host denial returns before any extension call. Each peer must explicitly return an allow boolean; malformed responses and crashes deny when the hook is mandatory. Explicit peer denial always denies, including for optional hooks. Optional execution errors are disclosed and cannot erase a prior denial. Caller cancellation stops the pipeline. Current policy input contains action/resource; detailed operation schemas and the Hand tool-policy adapter remain to be implemented.

The pipelines are tested with real subprocesses but are not yet installed into runtime generation or tool admission. Activation must preserve source trust classification when applying transformed context and must propagate mandatory errors to the action outcome. The transport and pipeline tests do not substitute for that integrated acceptance evidence.

## Transactional registry reload

`Manager` owns at most 16 peers. Its injected factory must verify the trusted launch digest and obtain execution approval, respect startup cancellation and bind process lifetime to the manager. The manager does not supply a permissive default factory or discover project-local code.

Reload runs only at an idle boundary. It snapshots and validates specifications, reuses healthy identical peers, stages changed peers and validates each handshake before replacing the active map. Failure closes staged peers and retains the prior map; a factory cannot return an active peer and trick rollback into closing it. A final health check precedes the registry swap. Once committed, removed/replaced peers are closed and joined. Cleanup errors after the swap return with `committed: true`, so callers can distinguish successful activation with cleanup failure from rollback.

Active calls hold the registry stable. Reload reports busy instead of interrupting them. Manager close cancels active calls before joining peers. Command registrations retain their extension namespace and cannot overwrite a built-in registration merely by sharing its name. The actual command dispatcher and terminal `/reload` adapter remain pending.

Transactionality covers the active registry and owned peer lifetimes. An admitted subprocess may perform external effects during startup; registry rollback cannot undo those effects. The product launcher must enforce the selected isolation/trust boundary. Real subprocess tests demonstrate healthy reuse, failed staging with the old peer still callable, replacement/removal cleanup, active-call protection, joined shutdown and rejection of factory reuse of an active connection.

## Typed manager operations

`ExecuteCommand` resolves the extension/name pair in the active registration set and rejects unknown commands before dispatch. Arguments are bounded to 16 KiB. Responses must contain explicit declarative presentation blocks and pass strict schema/control-character validation. `NotifyLifecycle` sends only registered subscriptions in host-defined order and accepts an empty acknowledgement object; observers cannot return an outcome override. Delivery failures are returned as diagnostics and mandatory errors. The application must keep already-terminal run outcomes immutable.

Manager `Transform` and `CheckPolicy` retain registry ownership for the entire pipeline so reload cannot replace a peer mid-operation. Host denial remains authoritative. Registry activation now requires the peer's declared capability set to equal its reviewed specification, not merely be a subset: otherwise a configured mandatory policy hook could silently disappear. The low-level handshake still checks a subset of a permission ceiling; the manager additionally enforces the reviewed feature contract.

These operations are exercised with subprocess fixtures, including invalid presentation/observer outcomes and missing configured policy capability. Application runtime/TUI/RPC adapters and executable trust admission remain outstanding; generic internal transport calls must not be exposed as an unrestricted public method-forwarding interface.

## Live question ownership

The `Questions` broker owns at most eight pending questions for a session. Each receives a random host token independent of the extension's reusable question ID. Read-only snapshots include extension provenance and copied options; responses require the exact live token and a schema-valid answer. A replay, expired token or answer to a prior instance cannot affect a later question. Cancellation removes pending state, and close releases waiting handlers. The callback handler captures the admitted extension identity instead of accepting one from callback input.

Answers are extension data and never modify permission grants. UI/RPC adapters must display provenance and keep these responses separate from approval controls. Those adapters are not implemented yet. Questions currently inherit the owning connection/manager request deadline, including the existing 30-second ceiling; the product interaction layer still needs an appropriate explicit interactive timeout policy. No pending question survives a process/session restart as an implicitly approved response.

Broker tests and a real extension question callback verify correct delivery, snapshot isolation, token replay protection, capacity and close/cancellation cleanup. This is backend evidence, not a completed interactive end-to-end journey.

The host launch factory now requires a reviewed configuration and an admission callback. Review identity covers the executable, explicitly listed package resources, capabilities, arguments, environment, workspace and disclosed host boundary. Factory construction independently validates resource paths and bounds even when a stored digest matches. Launch copies and rehashes the reviewed files into a private directory outside the workspace before starting the process; package argument references resolve into that snapshot. Shutdown joins process cleanup and removes the snapshot, surfacing transport cleanup failures. This is unrestricted host execution with explicit environment, not an OS sandbox or a snapshot of dynamic runtime dependencies. Container selection must use a separate factory and must never silently fall back to this one. Concrete application approval and UI integration remain required.

Application ownership now wraps extension commands and reload with the same cancellable reservation used by Hand operations. The application ExtensionHost configures trusted package identities before use; state callbacks resolve the currently selected session under controller locking. Questions use the host broker and can be answered while the command owns the application; a session switch or reload must wait until that command joins. Resource methods remain unavailable until their policy adapters exist. The application creator must close ExtensionHost before closing the runtime/session. Startup and front-end registration are still required; constructing this adapter alone does not activate project extensions.

Framing qualification treats input and output limits independently. An accepted
JSON frame can grow when the Go encoder escapes characters such as `<`. The
writer must reject any resulting frame over 256 KiB before writing bytes. The
reader fuzz oracle now measures the actual re-encoding instead of requiring all
accepted frames to fit after encoding, and checks successful writer output with
the reader. A permanent boundary regression explicitly crosses the encoded limit.
This corrects a test assumption without changing protocol limits or relaxing
validation.

Reviewed startup now connects context-transform capabilities to Harness's
request-only tool-text hook. The adapter partitions inputs into 64-item/64-KiB
batches and preserves host-owned indices. Enabled transforms reject any single
contribution over 16 KiB visibly; without active transforms the adapter imposes
no extension limit. Extension transformations affect provider working context,
not raw session results or protected user/system/assistant messages. Existing
host transforms execute first. Real peer tests cover both execution APIs and a
130-item batch boundary. Lifecycle and policy attachment remain separate work.

Tool policy attachment wraps the runtime permission checker, preserving the host
check and tool-list filtering first. Extension checks cannot reverse host denial.
Only host-allowed actions dispatch `policy.check` with `action: tool.execute`,
`resource: TOOL_NAME` and strict JSON `input` bounded to 16 KiB. No policy-capable
peer means no extra input restriction. Mandatory failure and explicit denial
produce the ordinary recorded tool-denial result before execution. The included
policy example compares one exact input path as a protocol demonstration; it is
not a replacement for host canonical path enforcement.

Run lifecycle subscribers now receive run.start with session_id and run.finish
with session_id/reason through both Harness execution APIs. Start delivery runs
after existing host admission; mandatory failure aborts before provider work.
Finish delivery uses the bounded cleanup context and reports failures without
rewriting the outcome. RunTurn emits a pair for each invocation, which can end
with reason continue; it does not pretend that one turn is a completed logical
multi-turn goal. Session and compaction notifications remain separate work.

### Resource callback integration (development candidate)

The application now dispatches `file.read`, `file.write`, `network.fetch` and
`process.run` through its registered `read_file`, `write_file`, `web_fetch` and
`bash` tools. Each callback runs the host approval hook and permission policy
before execution, then the tool-result hook. Protocol capabilities remain
required independently. Callbacks have an overall two-minute deadline and inherit
cancellation from their owning extension request. A policy callback cannot
recursively request resources; extensions declaring `policy.check` currently
cannot use these resource callbacks, even outside their policy handler.

Inputs are strict JSON objects. File reads accept `path`; writes additionally
require `content` (at most 64 KiB) and optionally `instruction_digest`. Nested
instruction responses preserve tool metadata so an extension can display the
guidance and explicitly return its current digest. A missing or stale digest
leaves the target unchanged. `network.fetch` accepts an HTTP(S) `url` and optional
`headers` (32 entries, 8 KiB combined), without embedded URL credentials or header
line breaks. It retains the registered web tool's destination checks and output
format. `process.run` accepts `command` (at most 16 KiB) and optional `timeout` in
seconds (0 uses the tool default; maximum 120). Commands use the registered Bash
execution boundary and are shell commands, not an argument-vector API.

Successful protocol responses contain the complete registered `ToolResult`,
including tool-level errors and metadata. Text output is bounded to 64 KiB and
the encoded result to 128 KiB. A transport-level failure after a write, process
or network operation starts returns `mutation_outcome_unknown`: callers must
inspect effects before retrying. This includes a result-hook panic after a write
has committed. There is no general rollback or exactly-once execution guarantee.

Current development tests cover real extension-process file callbacks, approval
and capability refusals, mandatory policy vetoes, guidance metadata round trips,
actual local shell execution and cancellation, input rejection, and the web
tool's private-address refusal. Process and network callbacks are also exercised over real extension IPC,
including cancellation after a command writes its start marker. An opt-in
`HAND_TEST_PUBLIC_FETCH_URL=https://example.com` run verified an actual public
HTTPS response, nonempty content, status 200 and URL metadata through the
registered web tool. This free public fetch is development evidence, not a model
provider evaluation. Container-backed extension launch and full final candidate
qualification still require evidence.

### Reviewed container activation

`--review-extensions` accepts an optional top-level `container` object alongside
`version`, `snapshot_root` and `extensions`. Its fields are `docker` (absolute
executable path), `socket` (absolute local socket path), `image` (immutable
`sha256:` image ID), `writable` and `network`. Review generation fingerprints the
selected binaries and assets without starting Docker, probing the image or
executing an extension. The existing explicit approval digest covers the full
review output, including this boundary.

Application startup passes the selected runtime backend to extension activation.
Every container review must exactly match its image, Docker/socket paths,
workspace, network setting and workspace writability. Host reviews cannot run
inside a container runtime, and container reviews cannot run on the host.
Reviewed reload preserves the same boundary; a mismatched replacement is rejected
and the existing extension remains usable. Removing all extensions is supported.

The container factory mounts verified private copies of the executable and
assets read-only under `/harness-resources`; `${package}/...` arguments resolve
within that mount. Native executable bytes must target the selected container
platform, and dynamic libraries must be present in the selected immutable image.
The existing image-volume refusal, non-root user, disabled capabilities, resource
limits and network setting remain in force. There is no fallback to host launch.

Native development tests exercised the real Go tool-viewer example through
factory launch, application activation, command execution, unchanged-review
reuse, rejection of a broader network setting and removal. Final CLI/RPC/TUI
container journeys, interpreted package support and complete recovery/platform
qualification remain pending.

### Container package runtime approvals

The package commands `review-runtimes` and `review-extensions` accept
`--container FILE`. This strict JSON file contains `boundary` (the container
fields described above) and `interpreters`, a map from declared runtime names
(`python`, `node`, `ruby`) to absolute executable paths inside that image.
Unneeded mappings are rejected. For example, an image providing Python at
`/usr/local/bin/python3` uses that path for the `python` mapping. Interpreter
paths within workspace, scratch or resource mounts are refused.

`packages review-runtimes --container FILE` creates image runtime reviews without
executing Docker. Its `review_digests` map identifies the digest for each
package/runtime pair; every generated `approved_digest` is empty. After explicit
review, supplying those approved digests in the runtime review document permits
`packages review-extensions --container FILE --runtime-approvals REVIEWS` to run
bounded `--version` probes. Probes use the exact image and interpreter, disabled
networking and an empty private read-only workspace; package source is not loaded.
An unmet minimum version, missing approval, changed image or mismatched interpreter
mapping rejects launch review generation.

The resulting startup review is separately approved using its complete file
SHA-256, as with native extensions. Interpreted entrypoints remain verified
package files. Python launch uses the image interpreter with `-I -B`; native
entrypoints continue to use their reviewed executable bytes. The source package
can be removed after installation because launch resolves the retained verified
package object. Native development tests cover this Python path and CLI review
round trip. Node/Ruby native image journeys and final candidate qualification
remain required before broader compatibility claims.
