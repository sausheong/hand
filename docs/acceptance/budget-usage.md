# Run and session budgets

The development implementation supports token, monetary and wall-clock limits
for sessions and explicitly selected logical runs. Session limits apply across
branches and compaction. Run limits apply alongside session limits, including
continuation turns, retries and summarisation. No limit is enabled implicitly.
These features are not yet qualified against a clean release candidate.

## Decisions, persistence and exhaustion

Token and monetary decisions set **absolute ceilings**, including prior charges
and outstanding or uncertain reservations. They do not add an allowance or erase
history. A new ceiling must exceed committed charges. A request that cannot fit,
or a reported overrun, latches exhaustion; a smaller later request cannot silently
resume it. Continuing requires a new explicit decision.

A time decision starts a new wall-clock allowance immediately, including idle
time. Supported user allowances are 1–2592000 seconds (30 days). Restart preserves
the absolute deadline. Inspection and replay never renew it. Run deadlines also
cover Hand's completion validator. Wall-clock enforcement assumes a trustworthy
host clock; clock rollback is not defended against.

Run IDs contain 1–64 ASCII letters, digits, underscores or hyphens. Selection is
explicit and persists across prompts and restart. A new prompt does not reset
charges or switch to a fresh run. Configuring an ID does not select it; selecting
an ID does not grant capacity. A different logical run requires explicit
selection of another configured ID. The run journal records its budget ID
separately from the process-local event RunID.

Saved selections require exactly one lowercase `version` field and one lowercase
`id` field. Duplicate fields (including escaped aliases), unknown/case-alias
fields, unsupported versions and invalid IDs are rejected after restart. An
invalid latest selection does not fall back to an older selection or grant an
unrestricted run. Rejection preserves the saved records for inspection.

The shared Harness annotation envelope likewise requires exact, unique
`version`, `kind` and `payload` fields. Ambiguous envelopes fail ordinary,
exclusive and recovering session loads. Recovery must not discard such a record
as a truncated tail, since doing so could hide a budget or accounting annotation.
This hardening currently resides in the local Harness candidate and still needs
release qualification.

Every RPC mutation requires `confirmed: true`. Use a new request ID for an
intentional change. Replaying a successful old request returns its original
response without reapplying its decision or selection. Controls require an idle
application owner; active runs and conflicting operations are refused.

## Terminal controls

| Command | Effect |
| --- | --- |
| `/budget` | Inspect session token totals and uncertainty. |
| `/budget-tokens TOTAL` | Set or resume the absolute session token ceiling. |
| `/cost` | Inspect session monetary totals and compaction spending. |
| `/cost CURRENCY TOTAL strict\|advisory` | Set or resume the absolute session monetary ceiling. |
| `/time-budget` | Inspect the session deadline. |
| `/time-budget SECONDS` | Start a new session wall-clock allowance. |
| `/prices` | Show installed tariffs and provenance. |
| `/prices-review PATH` | Review a tariff file without installing it. |
| `/prices-confirm DIGEST` | Install the exact table displayed by the review. |
| `/run-budget` | Inspect the currently selected logical run. |
| `/run-budget ID` | Inspect another run without selecting it. |
| `/run-budget tokens ID TOTAL` | Decide its absolute token ceiling. |
| `/run-budget cost ID CURRENCY TOTAL strict\|advisory` | Decide its absolute monetary ceiling. |
| `/run-budget time ID SECONDS` | Start a new wall-clock allowance for that run. |
| `/run-budget select ID` | Select the configured logical run. |

For example, `/run-budget cost repair-1 USD 1.000000001 strict` uses exact decimal
parsing with up to nine fractional places. Scientific notation, negative amounts,
excess precision and overflow are refused. Monetary ledgers cannot change
currency. A token command never enables monetary or time limits.

## RPC controls

After the `hello` handshake, use these methods and parameter objects:

| Method | Parameters |
| --- | --- |
| `budget.tokens` | `{}` |
| `budget.tokens.decide` | `{"limit":100000,"confirmed":true}` |
| `budget.cost` | `{}` |
| `budget.cost.decide` | `{"currency":"USD","limit_nano":1000000000,"strict":true,"confirmed":true}` |
| `budget.time` | `{}` |
| `budget.time.decide` | `{"seconds":600,"confirmed":true}` |
| `budget.prices` | `{}` |
| `budget.prices.set` | A complete `prices` array and `confirmed: true`. |
| `budget.run` | `{}` for the selection, or `{"id":"repair-1"}` |
| `budget.run.tokens.decide` | `{"id":"repair-1","limit":100000,"confirmed":true}` |
| `budget.run.cost.decide` | `{"id":"repair-1","currency":"USD","limit_nano":1000000000,"strict":true,"confirmed":true}` |
| `budget.run.time.decide` | `{"id":"repair-1","seconds":600,"confirmed":true}` |
| `budget.run.select` | `{"id":"repair-1","confirmed":true}` |

RPC monetary amounts are integer billionths of the named currency; the example
above is one USD. `strict` must be supplied explicitly. Inspections return bounded
totals, attempt/uncertainty counts, compaction spending and deadline information,
not the full per-attempt history. The Harness ledger API provides that history.

## Admission and accounting

Each actual provider attempt reserves estimated input plus a finite positive
output cap before dispatch. Input estimation includes the effective system
prompt, messages, tool schemas and a fixed image allowance, using the Harness
UTF-8 bytes/4 heuristic. This is not exact token counting. Missing output caps are
refused under token admission.

Conditional durable appends prevent concurrent owners from spending the same
remaining capacity. Use an exclusively loaded disk session for cross-process
durability. Each ledger is bounded to 10,000 records and admission preserves room
for outstanding settlements. Run ledgers are separately namespaced in the same
session; reopening an ID preserves its charges and reservations.

Successful reported usage reconciles reservations. Missing usage and partial
failed/cancelled usage retain at least the reservation; interrupted requests stay
reserved across restart. Oversized settlements fail while preserving the
conservative reservation and a readable journal. Denial by a later guard releases
earlier reservations as `not_dispatched` before any provider call. Run and
session totals describe overlapping scopes and must not be added together.

Exhaustion stops new admission and cancels owned work. Stream-error reconciliation
joins started tools, preserves completed outputs and pairs cancelled/unstarted
calls. Empty and duplicate tool-call IDs are rejected. Provider billing lag,
estimation error and non-cancellable in-flight work can exceed an exact invoice
cap; the implementation does not promise one.

## Pricing and tariff review

A `PriceSnapshot` identifies provider, destination, model, currency, source,
version and validity interval. Input/output/cache rates are integer nano-currency
units per million tokens; fixed per-attempt charges are separate. Arithmetic
reserves the highest input/cache rate plus the output cap, subtracts cache subsets
from total input during reconciliation, rounds the combined charge upward once
and checks overflow. There is no currency conversion or implicit market pricing.

Strict mode refuses missing, stale or incompletely bounded tariffs. Advisory
mode records unknown prices explicitly; numeric zero does not mean free. Mark
charges unbounded when reasoning, image, tool, tiered-cache or other costs are not
represented by the snapshot and reported counters. Admission-time snapshots are
retained for reconciliation even after expiry. Historical unpriced advisory
attempts prevent silently switching the ledger to strict mode; an audited
reconciliation mechanism for those records remains unfinished.

Matching uses the exact provider, destination and model. Hand installs local
route metadata with the constructed client; omitted endpoints are labelled
`default endpoint for PROVIDER`. Independent summarisers retain their own route.
This metadata is excluded from request JSON and is not a credential or remote
attestation. Direct Harness callers must set `Runtime.Route` and
`Summarizer.Route` to match their clients.

Price tables allow at most 16 unique routes. Replacing or clearing the table is
explicit and does not alter previous charges, reservations or tariff snapshots.
Strict requests fail when no current matching tariff exists. Tariffs are
caller-supplied assertions, not independently audited billing.

For terminal review, use a regular, non-symlink JSON file no larger than 48 KiB,
with `"version":1` and an explicit `"prices"` array. Unknown fields, trailing data,
invalid tariffs and missing/null arrays are refused. `{"version":1,"prices":[]}`
explicitly reviews clearing the table. `/prices-confirm DIGEST` installs the
reviewed bytes, even if the source file changed afterward. Review state is
single-use and invalidated by session operations. The digest identifies content;
it is not a signature or tariff audit. No real tariff or paid-call authorisation
is supplied by this document.

## Outcomes and embedding

Exhaustion emits `budget_exhausted`, remains unverified and uses exit code 4.
Reasons distinguish `session_token_budget`, `session_cost_budget`,
`session_time_budget`, `run_token_budget`, `run_cost_budget` and `run_time_budget`.
Strict unknown pricing reports `cost_price_unknown`. A missing price needs an
explicit tariff correction; exhaustion needs an explicit allowance decision.
User cancellation remains a separate outcome.

Harness exposes session decisions through `DecideTokenBudget`, `DecideCostBudget`
and `DecideTimeBudget`, and corresponding per-run decision methods. Owners call
`PrepareRunBudget` once for a configured stable ID and carry the returned context
through execution, continuation and compaction, cancelling after child work joins.
Inherited guards remain attached. Repreparing the same scope does not reserve
twice. Newly enabled dimensions or changed deadlines invalidate stale bindings;
prepare a fresh context from the owner after those decisions. Hand's shared
Service performs this binding for the selected run. Direct SDK/embedding callers
must preserve the same ownership and context contract.

## Development evidence and remaining qualification

Built CLI runners use fresh output directories and local scripted HTTP providers:

```sh
python3 scripts/check_budget_rpc.py --hand-binary /absolute/path/to/hand --out /absolute/new/session-token-evidence
python3 scripts/check_cost_rpc.py --hand-binary /absolute/path/to/hand --out /absolute/new/session-cost-evidence
python3 scripts/check_time_rpc.py --hand-binary /absolute/path/to/hand --out /absolute/new/session-time-evidence
python3 scripts/check_run_budget_rpc.py --hand-binary /absolute/path/to/hand --out /absolute/new/run-budget-evidence
python3 scripts/check_run_time_rpc.py --hand-binary /absolute/path/to/hand --out /absolute/new/run-time-evidence
python3 scripts/check_run_budget_terminal.py --hand-binary /absolute/path/to/hand --out /absolute/new/terminal-evidence
```

RPC journeys cover refusal before dispatch, explicit resumption, restart,
replay protection, reported charges and actual deadline stream disconnection.
See [session tokens](budget-rpc.json), [session cost](cost-rpc.json),
[session time](time-rpc.json), [run accounting](run-budget-rpc.json) and
[run time](run-time-rpc.json). The [140×45 PTY journey](run-budget-terminal.json)
checks rendered run controls, persisted decisions and clean exit. These runners
make no paid calls; the PTY control journey sends no model prompts.

Permanent regressions additionally cover concurrency, pricing arithmetic,
compaction, stale bindings, malformed provider IDs and owned-process cleanup.
See [run binding](harness-run-binding.json), [Hand ownership](run-owner-integration.json),
[stream reconciliation](harness-stream-reconcile.json) and
[tool identity](harness-tool-identity.json).

Remaining work includes broader session-budget and tariff-review PTY journeys,
audited reconciliation of unpriced history, exact-candidate coverage/performance,
native platform qualification, released Harness integration and authorised live
evaluation. Development tests and single timing observations do not satisfy
those final gates or establish superiority to Pi.

## Persisted usage schema integrity

Version-1 request-usage and tracking annotations use exact, unique canonical field names. Duplicate names (including escaped duplicates), case aliases, unknown fields and missing required fields are rejected before accounting. Reported usage requires explicit non-null integer `input_tokens` and `output_tokens`; cache counters may be omitted but cannot be null. An explicit `usage: null` with source `unavailable` retains unknown accounting rather than zero consumption. The reader reports malformed stored accounting and the writer refuses to append over it, preserving original bytes. See [integrity regression evidence](usage-integrity.json).

Harness budget journals apply the same ambiguity rule recursively to token/cost/deadline events and nested price snapshots. Canonical lowercase keys must be unique; explicit null scalar values are rejected rather than becoming zero or false. This applies to session-wide and logical-run ledgers. Corrupt token/cost history prevents new reservation/decision appends and remains unchanged for inspection. [Development evidence](harness-budget-json.json) records reopened-session and integration checks.

Required budget fields are derived from the version-1 writer schema: every field without `omitempty` must be present, including nested quote and price fields. Explicit zero remains valid; omission cannot silently manufacture a zero settlement or free tariff. Optional omitted fields remain compatible. [Required-field evidence](budget-required-fields.json) includes a reproduced 20-token reservation released by a missing charge before this fix.
