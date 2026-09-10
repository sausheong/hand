# Paired evaluation executor: remaining implementation contract

Status: design informed by source review, not an implemented or authorised executor.
Date: 9 September 2026.

Implemented component: `scripts/evaluation_pi_events.py` now observes a bounded
RPC stream for one fresh run. Feed raw byte chunks to `PiRunObserver`; use
`ready_for_stats` to decide when to request the separately correlated final
statistics, then `finish()` to obtain an `observed_unverified` or `incomplete`
report. It separates reported cost from verified billing. The executor must
still supply actual Pi I/O, timeout/cancellation, process joins and spending
enforcement. Synthetic observer tests do not qualify the Pi runtime. Evidence:
[pi-observer.json](../pi-observer.json).

The separate `scripts/check_pi_offline.py` probe now exercises the published
0.85.1 npm runtime with a deterministic custom provider, fresh homes/workspaces,
and no model credentials. Success, automatic retry and abort all passed; actual
event streams are retained as observer regression fixtures. Package integrity
matches registry metadata. This verifies those local RPC behaviours, not the
complete comparison adapter or spending boundary. See [pi-runtime.json](../pi-runtime.json).

## Competitor candidate

The official site links to `earendil-works/pi`. The release reviewed here is
v0.85.1, commit `d981de1229ef899957bbe968bc8dcda02a21f477`. Its package is
`@earendil-works/pi-coding-agent`, with Node >=22.19.0 and CLI entry
`dist/bundle/cli.js`. This is a proposed comparison pin; local source, dependencies
and executable bytes still need qualification. See [source-review record](../pi-source-review.json),
[official site](https://pi.dev),
[release](https://github.com/earendil-works/pi/releases/tag/v0.85.1) and
[pinned package metadata](https://raw.githubusercontent.com/earendil-works/pi/d981de1229ef899957bbe968bc8dcda02a21f477/packages/coding-agent/package.json).

## Lifecycle requirements

Pi's prompt response acknowledges admission, not completion. `agent_end` can
precede retries or continuations; `agent_settled` identifies completed automatic
activity. `get_session_stats` includes assistant, tool-reported, and summarisation
usage. Queue clearing and abort are separate operations. These documented
distinctions must be tested against the actual pinned runtime.
[Release RPC documentation](https://raw.githubusercontent.com/earendil-works/pi/v0.85.1/packages/coding-agent/docs/rpc.md).

The executor must implement and test:

1. Start each arm with a fresh isolated home, workspace and session. Record
   resolved model identity, settings, tools, source trees and executable digests.
2. Correlate prompt admission with its request ID. Rejection is a failed attempt.
   Never start another scheduled task in that session.
3. Continue collecting events through retry, compaction and queued continuations.
   Do not infer success from a transport acknowledgement or intermediate end.
4. At settlement, collect accounting, close and join the process tree, preserve
   its workspace changes, and run the independent frozen verification fixtures.
   Agent completion is distinct from task success, which also requires review.
5. On timeout, cancellation or budget exhaustion, revoke access to the provider
   first, clear queued work, abort and join. Retain the failed attempt in the
   denominator. Missing final usage remains uncertain rather than zero.

## Spending enforcement

Implemented quote component: `scripts/evaluation_quote.py` accepts an explicit
versioned contract with provider/model identity, source digest, context/output
limits, all applicable complete token-price sets, and a maximum per-request fee.
Rates are exact decimal USD strings per million tokens. It reserves the complete
input context at the highest input/cache rate, plus the enforced output cap at
the highest output rate and the fee, rounding upward to integer billionths of a
USD. Missing categories, unknown billing semantics, model mismatch, impossible
output caps and numeric overflow are rejected. Contract hashes bind all inputs.

This method supports text-token billing with input/cache counts normalised into
disjoint categories. It depends on verified provider limits and complete pricing
tiers/fees. It does not establish those facts itself. The gateway must reject
unsupported billing, payload types, service tiers or endpoint/model changes and
enforce the quoted output limit on the forwarded request. A full-context quote
may exceed a budget even for a short prompt. Do not silently reduce it: obtain
an independently justified tighter bound or size the approved budget accordingly.
No actual provider catalogue or spending approval is supplied by this component.

Implemented body-validation component: `scripts/evaluation_anthropic_request.py`
checks bounded, unambiguous UTF-8 JSON for `POST /v1/messages`, exact model identity,
text/custom-tool content, cache policy, thinking and output controls. Unknown fields,
multimodal content and server-billed tools fail before reservation. `admit_request`
validates the body and uses `reserve_quoted` to bind its unchanged bytes and requested
output cap to the complete quote. No provider request is sent by this module.

Actual provider serialization probes now capture text, tool-roundtrip and thinking
requests from installed Pi AI 0.85.1 and Hand's current local Harness provider,
using injected fetch/RoundTripper implementations that return HTTP 400 without
network access. All six bodies pass unchanged. Pi's SDK emits exactly
`/v1/messages?beta=true`; this spelling is now allowed alongside `/v1/messages`.
Other query strings and paths remain rejected. Captured bodies are permanent
regression fixtures. These probes do not run full coding-agent sessions or prove
all model/default combinations compatible. See [serialization evidence](../provider-request-capture.json).

Header/beta policies, upstream origin, redirects, HTTP framing, authentication,
traffic isolation and response accounting remain gateway work. Preserve observed
SDK options; do not strip or disable agent defaults to make validation pass.
See also the original [body-validation evidence](../evaluation-anthropic-request.json),
which retains the earlier narrower validation scope.

Implemented HTTP envelope component: `scripts/evaluation_http_policy.py` consumes
raw header pairs before duplicate merging. It requires the gateway authority,
exact body length, JSON media type, reviewed API version and a constant-time match
against the opaque run key. Unknown fields, transfer/content encoding, duplicate
names, destination overrides and unreviewed beta combinations fail. Beta sets
must be explicitly supplied by the frozen policy; token spelling is not evidence
of billing semantics. SDK telemetry and reviewed protocol fields are preserved.
Gateway credentials, Host, Content-Length and Connection are excluded from the
upstream header projection; the HTTP sender must generate framing and supply the
provider credential for its fixed upstream.

All six actual SDK header sets pass with injected HTTP framing and a test run key.
The combined `admit_http_request` path now validates headers and body before
quoted admission. Run revocation/expiry and budgets are checked by the ledger;
transport metadata, quote and numerical reservation commit atomically. The fixed
HTTPS Anthropic origin, exact target, reviewed headers/beta combinations and body
digest are retained without credentials. Reopen validates their consistency.
Transport reads are evidence, never replay permission. Tests cover six actual SDK
requests, a process crash, injected storage failure and corrupted body identity.
See [combined admission evidence](../evaluation-http-admission.json).

The trusted caller must still resolve run credentials and frozen configuration,
parse the HTTP socket, forward exact bytes through the fixed upstream, enforce
network isolation and reconcile authentic usage. These remain mandatory. See [header-policy evidence](../evaluation-http-policy.json).

Implemented storage component: `scripts/evaluation_budget.py` provides SQLite
reservations bound to one manifest digest. Each forwarding attempt needs a new
request identity; the transaction commits before admission returns. Outstanding
or uncertain calls retain their full input/output/cost bounds after restart.
Known usage releases only the unused reservation; request slots remain consumed.
Revocation and observed expiry stop further admission permanently, including after clock rollback. Stored limits and reservation states are validated on open and before writes; malformed records fail closed. Conflicting usage or an exceeded
bound persist an audit record and block all new reservations. Amounts use integer
billionths of a USD, avoiding floating-point balance arithmetic.

Quoted admission now also stores the SHA-256 of the exact outbound body bytes,
the complete canonical pricing contract and provider/model identity in the same
transaction as the reservation. `reserve_quoted` returns only after both records
commit; `binding` retrieves reconciliation evidence and never grants permission
to replay a request. Reopening and subsequent writes recompute the quote and
reject a mismatch with the stored bounds. Request bodies and credentials are not
stored. A raw numeric `reserve` remains a lower-level primitive and carries no
such binding; the future provider gateway must use quoted admission.

The caller must validate the actual provider payload and endpoint and forward
exactly the hashed bytes with the quoted output limit. This storage change does
not perform that validation or authenticate pricing. Legacy development ledgers
without the binding/transport tables are rejected; preserve them for reconciliation rather
than deleting/recreating a budget. No paid ledger has been migrated or reset.

This is not an authorisation mechanism. The caller must verify approval, bind run
IDs to the frozen schedule, establish defensible bounds, enforce monotonic request
timeouts and isolate the ledger from agents. It must also authenticate usage
evidence before settlement; a digest alone does not prove billing. The ledger
does not forward requests, price models, enforce network routing or automatically
resolve an ambiguous charge. Those gateway responsibilities remain unfinished.

The release supports provider `baseUrl` overrides, which can route requests
through an evaluation gateway without redefining all models. Configuration alone
does not prove that every request is intercepted or bounded.
[Release model documentation](https://raw.githubusercontent.com/earendil-works/pi/v0.85.1/packages/coding-agent/docs/models.md).

Implement a shared boundary for both arms, with provider credentials held outside
agent workspaces. Agent containers must have no alternative provider route or
inherited credentials. The gateway must enforce the approved aggregate cap and
per-run request, token, cost and time caps before forwarding requests. It must
reserve a defensible upper bound on each request, including output/reasoning and
applicable input/cache pricing, and reject unknown pricing or unbounded requests.
Do not treat an approximate local token count as a proven input bound.

Record reservations durably before forwarding. Interrupted or ambiguous requests
retain their reserved amount until reconciled. Retries consume new reservations;
restarting an executor must not reset a budget. Summary and nested tool calls
must pass the same boundary. Provider usage, gateway records and agent-reported
totals must be reconciled without double-counting cache tokens.

This requires real fault tests: concurrent final-budget requests, crash after
forwarding, missing usage, retry after timeout, disconnect during streaming,
attempted endpoint bypass, restart with a pending reservation, and revoked run
credentials. A final-statistics parser or process timeout cannot replace these
controls. No gateway or paid-run authorisation is supplied by this document.

## One-time outbound dispatch

`scripts/evaluation_forward.py` now verifies the exact body against durable
transport evidence, then commits `claim_dispatch` before creating an upstream
connection. Expired/revoked runs, poisoned budgets and duplicate dispatches fail
before connection. A crash after claim never permits reusing that request ID,
even when no upstream bytes can be proven sent. The reservation is retained.

The default sender uses verified TLS to the fixed `api.anthropic.com` host, injects
an explicitly supplied provider key, preserves the admitted body and reviewed
headers, and never follows redirects or retries automatically. Response bodies
are streamed to a supplied evidence sink under a byte cap. A watchdog interrupts
connected sockets on deadline/cancellation, including stalled headers. Truncated
Content-Length bodies fail. Returned metadata excludes provider credentials and
routing headers such as Location. Every dispatched request remains uncertain;
this component cannot settle billing from HTTP status alone.

Nine tests include six real HTTP socket-pair cases and process-crash recovery.
No external connection or live TLS handshake was made. DNS/connect delays require
separate deployment qualification; the gateway controller must wire run revocation
to in-flight cancellation. Incoming HTTP serving, trusted schedule/token resolution,
network isolation, authentic usage reconciliation and live approvals remain open.
See [sender evidence](../evaluation-forward.json).

## Response usage observation

`scripts/evaluation_anthropic_usage.py` now validates complete unencoded JSON or
SSE Messages responses. SSE must contain one model-bound message, balanced content
blocks, final cumulative output usage and message_stop. Explicit initial uncached,
cache-read, cache-write and output counters are required; omission never means zero.
Known cumulative updates must not regress. Cache-creation breakdowns must sum to
the aggregate, and unknown billing fields/service tiers or server tools fail closed.
Input totals add the disjoint provider counters once.

`verify_observed_usage` first checks successful complete sender status, byte count
and body digest. Encoded responses require a future bounded decoding step; the
sender now retains Content-Encoding so compressed data cannot be mistaken for
plain SSE/JSON. Results are `usage_observed_unverified` or `incomplete`, never a
ledger settlement. A socket-pair integration test retains the full reservation
after observing usage: provider origin, applicable prices/fees and reconciliation
still need evidence. No real provider usage was collected in these tests.
See [usage evidence](../evaluation-usage.json).

## Trusted run controller

`scripts/evaluation_gateway.py` now composes the request path. `GatewayController`
opens an existing manifest-bound ledger and snapshots explicit run/token/model,
pricing and beta settings; it never creates budgets or discovers credentials.
Requests resolve their opaque key against trusted run settings. Clients cannot
provide a run ID or pricing/model policy. Each accepted attempt receives a fresh
identity and is admitted before single-use forwarding.

Response bytes are written into a fresh private evidence directory before any
optional downstream sink. Reports bind the actual evidence-file digest to the
sender observation. Revocation commits in SQLite while admission is excluded,
then signals every active transport for that run. Controller close rejects new
requests and signals active work; callers must join their request workers.

Five socket-pair integration tests cover frozen settings, rejection, concurrent
budget enforcement, active revocation and close cancellation. No incoming HTTP
listener, full coding-agent session or real provider was used. The HTTP server,
streaming response-header bridge, external approval/configuration attestation,
network isolation and billing reconciliation remain required. See
[controller evidence](../evaluation-gateway.json).

## Incoming HTTP and streaming bridge

`scripts/evaluation_http_server.py` provides an HTTP/1.1 handler and a threaded
listener with explicit connection slots and timeouts. The handler validates raw
headers and authenticates the run key before reading the bounded body. An absolute
intake watchdog closes stalled headers/body reads. Duplicate/ambiguous framing,
unsupported methods/routes and Expect negotiation are rejected. Each socket serves
one request and closes; apply and measure this transport policy equally to both arms.

Sender response headers now reach the handler before body collection. Each captured
chunk is bridged immediately with HTTP chunk framing. The final zero chunk is sent
only for a complete upstream observation; truncation/cancellation closes incomplete
so clients cannot infer success from a partial body. Gateway errors are generic JSON
and default access logging is disabled to avoid recording request credentials.

Five tests use actual HTTP bytes over socket pairs on incoming and upstream sides,
including incremental delivery, truncation, duplicate length and stalled headers.
Three additional real loopback TCP tests now pass: round-trip forwarding, extra
connection refusal without another handler, and joined shutdown of stalled intake
with all slots returned. The initial sandbox bind denial is retained separately
from the subsequently approved local execution. See [listener evidence](../evaluation-listener.json).
Actual SDK network integration and deployment isolation still need qualification. No external provider
was contacted. See [HTTP handler evidence](../evaluation-http-server.json).

## Actual Pi SDK over the gateway

`check_pi_gateway.py` and `check_pi_gateway.mjs` now run installed Pi AI 0.85.1
through the real loopback TCP gateway using its actual fetch transport. Fetch is
guarded to the single local origin and rejects redirects; the gateway upstream is
a socket-pair fixture returning deterministic SSE. Text, tool-roundtrip and thinking
cases all complete with the expected SDK text/usage and joined server workers.

The first run exposed Node-added `accept-language: *` and `sec-fetch-mode: cors`,
which were absent from pre-network captures. Policy rejected the request before
reservation. Those exact values are now permitted unchanged, with real captured
headers retained as regression fixtures. All three successful reservations remain
uncertain: synthetic fixture usage and prices never establish genuine billing.
This verifies the provider SDK path, not a whole Pi coding-agent session, Hand's
network path or container isolation. See [network probe evidence](../pi-gateway-network.json).

## Hand/Harness SDK through the same listener

The shared `check_pi_gateway.py --client harness` runner invokes
`evaluation/testdata/harness_gateway.go` against Hand's current local Harness
provider. Its HTTP transport permits only the configured loopback Messages origin,
disables inherited proxy routing and uses the actual Go SDK over TCP. Text,
tool-roundtrip and thinking cases all receive the expected complete streamed text,
end_turn and five input/two output fixture counters through the same admission,
forwarding and response bridge used by Pi.

Actual Go network headers, including its automatic gzip negotiation, are permanent
regression fixtures. The Pi branch also passed again after sharing the runner.
All requests retain their full uncertain reservations. These are provider SDK
integration probes with constructed contexts, not full Hand/Pi coding-agent sessions,
genuine provider usage or a published Harness candidate. See
[Harness network evidence](../harness-gateway-network.json).

## Fairness and integration

Apply the same external enforcement to both default and configured arms. Preserve
their actual product defaults within those explicit experimental limits and
report all deviations. Do not silently disable one agent's retries or compaction
to simplify accounting. Freeze configurations before held-out runs; tuning after
inspection requires a new comparison design, not relabelling exposed tasks.

Reuse the existing schedule, checkout, fixture-integrity, bounded-process and
result-reporting modules. Connect them only after actual runtime adapters,
isolation and gateway controls pass. The current five-task calibration catalogue
is still too small and has no held-out tasks. The required 30-task corpus, ten
held-out tasks, both modes, three repetitions and independent scoring all remain
mandatory. Native-provider smoke tests remain a separate acceptance requirement.

## Pi CLI gateway tool-turn probe

`check_pi_gateway.py --client pi-agent` launches the pinned actual Pi CLI via
`check_pi_agent_gateway.py`, observes RPC settlement and correlated statistics,
and checks a successful read tool and its marker in the next provider request.
The fixture uses synthetic upstream responses, read-only tools, thinking off and
a 4096 output cap. These settings are probe-specific, not comparison defaults.
Initial fixture failure and successful rerun are retained in
[Pi agent evidence](../pi-agent-gateway.json). The captured stream regression
rejects missing tool completion; it does not score coding quality or settle usage.

## Hand CLI gateway tool-turn probe

`check_pi_gateway.py --client hand-agent --runtime /absolute/path/to/hand`
launches the compiled Hand CLI using a fresh HOME and explicit gateway profile.
It requires an actual read_file result, fixture text and a final completed JSONL
event with successful exit. The shared runner checks two admitted requests,
complete usage observations, retained uncertain reservations and joined workers.
The output cap 4096 and three-turn limit are probe settings. See
[Hand agent evidence](../hand-agent-gateway.json); this does not establish live
quality, container isolation or a released dependency.

## Hand event observation

`evaluation_hand_events.HandRunObserver` consumes bounded LF-framed JSONL
for one fresh invocation. It requires contiguous event sequences, unique IDs,
stable session/run/request identity, paired tool calls/results and a final
recognised terminal outcome. Missing or malformed records remain incomplete.
Tool errors and failed/cancelled outcomes remain visible; observed completion
is not independent task correctness. The caller must separately establish
process cleanup, binary/model identity, gateway usage and verified patch scoring.
See [observer evidence](../hand-observer.json).

## Supervised Hand invocation

`evaluation_hand_run.run_hand` combines explicit argv/environment/workspace,
bounded separate process logs and group cleanup with the Hand event observer.
A recognised terminal must match the actual exit code, and the process and
pipes must join. Timeout, cancellation by the supervisor, output limit and
cleanup failure remain incomplete even if stdout claims completion. Distinct
Hand failure outcomes can be observed without being scored as task success.
The caller owns configuration/approval and independent patch verification.
See [supervision evidence](../hand-supervision.json).
