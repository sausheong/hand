# Hand: definition of done and completion evidence

> Remaining scope was explicitly revised by the user on 9 September 2026 to execution isolation and Harness release integration. See the [scope amendment](scope-amendment-2026-09-09.md). The original requirements below remain historical; omitted work is not a pass.

Date: 8 September 2026  
Applies to: [implementation plan](../2026-09-07-hand-implementation-plan.md)  
Status: acceptance contract; most required product tests do not exist yet

## 1. What counts as complete

**A feature being present, a milestone checkbox, a model saying “done”, and a green unit suite are insufficient.**

The full implementation goal is complete only when all of the following hold:

1. Every plan work package and its individual implementation bullets map to shipped code or a documented conditional decision, executable checks and evidence.
2. Every required scenario in the [test matrix](test-matrix.md) passes against the same clean Hand candidate and pinned, released Harness dependency. There are 32 requirement groups and 221 scenarios, including global quality gates.
3. The quantitative gates below pass, required platform and fault tests run without skips, and integrated user journeys pass.
4. A final acceptance review confirms that tests assert the intended behaviour, examples work, no placeholder implementation remains, and no defect violating an acceptance criterion remains open. There must also be no open P0/P1 defect.
5. Live task evaluation meets the product-quality floor and reports the comparison with Pi honestly. A negative/inconclusive comparative result can complete an evaluation; it cannot justify a superiority claim.
6. A checksummed release candidate and migration/rollback documentation exist for the tested commit. Publishing Hand publicly is not required by this goal. A released Harness dependency is required where shared fixes are needed.
7. The full evidence checker returns `status: complete`, and the executing goal has verified the evidence's actual origin and meaning as described below.

All required work remains required when inconvenient or externally blocked. Missing CI, a container runtime, a platform runner, credentials, budget or dependency release produces a pending gate, never an implicit pass.

### Completion levels

| Level | Meaning | Can the full implementation goal finish? |
|---|---|---|
| `not_complete` | Missing, failed, skipped, stale or invalid evidence | No |
| `offline_ready` | All implementation, platform, review and offline quality gates pass; live evaluation is not checked | No; report the remaining live gate |
| `complete` | Offline and live gates pass for one candidate | Yes, after evidence provenance/semantic review |
| Comparative advantage demonstrated | A separate, supported claim about a stated metric/workload | Not implied by `complete` |

This resolves the original plan's allowance to leave paid comparison pending: it permits an offline-ready checkpoint, not full completion. No paid calls are authorised by this document. The future goal should prepare the exact evaluation matrix and spending cap before requesting any missing budget authorisation.

WASM implementation, additional native sandboxes and a public marketplace remain conditional as in the plan. M7.D requires a reasoned decision. If WASM is judged necessary, its implementation and tests become required before M7.D passes; the decision cannot silently drop an accepted capability.

## 2. Test coverage required

### Quantitative gates

User-approved revision, 2026-09-09: the minimum is 80% for overall, changed-production and every critical-group statement coverage. This supersedes the earlier 90% changed/critical thresholds. Scenario coverage remains 100%; all other acceptance gates remain required.

| Metric | Required threshold | How to establish it |
|---|---|---|
| Requirement/scenario coverage | 100% of required scenarios | Every manifest scenario has a passing record, real oracle and hashed raw evidence |
| Hand production statement coverage | At least 80% | Uncached Go coverage profile, all production packages, atomic mode |
| Changed production statement coverage | At least 80% | Diff-aware instrumented-statement report against recorded baseline `e6dd263`; new files included |
| Critical subsystem statement coverage | At least 80% per group | Lifecycle, permissions, persistence, execution, checkpoints, budgets, protocol, extensions and accounting; include changed Harness code |
| Required skips | 0 | Parse test events, including integration/build-tag/platform suites; no missing tests reported as success |
| Acceptance defects / P0-P1 defects | 0 / 0 | Issue/defect inventory and final review |
| Critical concurrency/fault scenarios | 20 consecutive repetitions, no failure | Cancellation, stale results, writer ownership, reservations, approval races and extension reload |
| Parser fuzzing | At least 60 seconds per target | Session records, RPC frames, hook payloads, tool inputs and package manifests; preserve failures as regression corpus |
| Long-transcript key latency | p95 ≤100 ms | 10,000 blocks, at least 1,000 input events across 30 runs on recorded reference hardware |
| Cancellation initiation | p95 ≤1,000 ms | 30 trials per cancellation path; UI action to backend cancellation signal |
| Child cleanup | Every child gone within 5,000 ms | Actual process-tree probes on Linux/macOS; start clock at cancellation, include grace/kill escalation |
| Live task success | Hand ≥80% overall and ≥80% held-out | Verified scoring on pinned tasks; cancelled, exhausted and failed runs count as failures |

These are acceptance thresholds chosen now, not historical measurements. Do not lower a threshold or remove a test just to obtain a pass. A justified change requires an explicit scope/acceptance decision from the user, a versioned contract update and fresh evidence.

Statement coverage does not measure assertion quality or branch coverage. Critical tests must assert effects on disk/processes/network, state invariants, error outcomes and absence of forbidden effects. Reviewers must reject tests that merely repeat implementation calculations. For the four original bugs, show that the regression tests fail with the defective implementation and pass after the fix. Use fault injection to demonstrate other negative-path oracles.

For coverage, exclude tests and machine-generated code only, with an enumerated reviewed exclusion list. Do not exclude poorly covered code. Record package-to-critical-group mappings; unimplemented or unmapped critical groups cannot report 100%. A UI-only diff may have no changed instrumented statements: report a justified no-applicable-statements result in the raw report, and review the UI journey tests; do not invent statement counts. The aggregate plan has substantial changed production code and still requires the numeric changed-code gate.

Memory/output tests must assert configured capture limits plus a documented bounded buffer overhead, and disk-spill limits including retention. Record the actual caps in evidence. For startup, require no-MCP p95 latency no worse than M0 baseline ×1.2 +50 ms on identical hardware/configuration; initial launch must not wait on optional MCP. Evidence for these non-scalar checks belongs to G-PERF scenarios.

### Platforms and test layers

- Unit/property tests: calculations, parsing, state transitions, policy matching and invariants.
- Integration tests: real files, permissions, processes, local HTTP/MCP fixtures, actual persistence and compiled clients. Fake providers are suitable for deterministic application semantics, not model quality.
- End-to-end tests: launch the built binary in a PTY or subprocess and exercise public commands. An in-process TUI model test alone is insufficient for G-JOURNEY.
- Native runtime suites: Linux and macOS. Compile all four advertised OS/architecture targets and smoke-test each archive on native or correctly emulated runners; record emulation. Cross-compilation alone is not runtime evidence.
- Isolation: real container boundary tests on Linux/macOS with a controlled sentinel file and test network endpoint. A missing container runtime is pending, not skipped/pass.
- Fault tests: deterministic disk-full/permission failures, process interruption, truncated/corrupt records, replay, stale async results and output flooding.
- Live providers: native Anthropic/OpenAI/Gemini paths, an aggregator and a local endpoint actually execute representative tool loops, streaming, cancellation and usage reporting. Unsupported capabilities must fail clearly. Use authorised credentials and spending only.

Future M0 test runners must inventory expected tests before running them, collect structured per-test events, fail if selection matched zero tests, and prevent `go test -run` filters or build tags from hiding required suites. Full release runs use `-count=1`, not cached success. Rerunning until green does not erase a failure; fix it and record a new clean run.

Illustrative existing Go commands (not substitutes for future integration suites):

```sh
go build ./...
go vet ./...
go test -count=1 -race ./...
go test -count=1 -covermode=atomic -coverprofile=/tmp/hand.cover ./...
go tool cover -func=/tmp/hand.cover
```

Formatting must be checked without mutating source. Future suite orchestration and diff-coverage reporting must be implemented in M0; they are not supplied by the completion checker.

## 3. Live evaluation protocol

G-LIVE requires at least 30 tasks, at least three independent paired repetitions per task, two agents (Hand and Pi) and two modes (defaults and reasonably configured). This is at least **360 task runs**, excluding provider smoke tests and retries. Reserve at least ten tasks as held-out before prompt/tool tuning. Score Hand's overall and held-out success without discarding failed or budget-exhausted runs.

Pin agent versions, task/repository snapshots, provider/model/reasoning settings, per-run budget, tool access, configured-mode packages, scoring checks and random seeds where available. Run in fresh isolated workspaces; counterbalance execution order. Keep raw outputs, diffs, tests, costs, human interventions and failure classifications. Log any unavoidable configuration mismatch.

Use repository tests plus review of the actual requested change as the task oracle; tests that pass without accomplishing the task do not establish success. Compute paired differences and uncertainty at the task level rather than treating repeated runs as independent tasks. Report default/configured modes separately. Publish cost per successful task alongside failures and task success, so cheap failure cannot masquerade as efficiency.

`comparison_conclusion` is either `advantage_demonstrated` or `not_established`. The first needs a predeclared metric and supporting uncertainty analysis; the second is valid if Hand meets its quality floor but comparative advantage is not established. “Better than Pi overall” requires a separately defined and supported objective, not this checker alone.

## 4. Evidence format and runnable checker

The following files are supplied now:

- [requirements.json](requirements.json): required IDs, evidence categories and scenarios; no success flags.
- [test-matrix.md](test-matrix.md): human-readable coverage matrix.
- [check.py](check.py): a standard-library Python evidence validator.
- [test_check.py](test_check.py): synthetic positive/negative tests of the validator, not product acceptance evidence.

Run from the repository root:

```sh
python3 docs/acceptance/check.py
python3 docs/acceptance/check.py --profile offline --evidence /absolute/evidence/run.json
python3 docs/acceptance/check.py --profile full --evidence /absolute/evidence/run.json
```

The first command reports missing evidence and exits nonzero today. Exit 0 means the requested profile's evidence validates; always inspect the JSON `status`, because offline exit 0 is **not** full completion. Exit 1 means unmet gates; exit 2 means malformed/inaccessible evidence or checker error. A Stop hook must map any non-complete result to “continue/pending”, without starting uncontrolled retry loops for external prerequisites.

Evidence must be outside the candidate checkout. Freeze a clean commit before the final run, record its SHA and released Harness version, and capture SHA-256 hashes of this plan and the requirements manifest. Any source/dependency/contract change invalidates earlier final evidence. Include schema v1, the following top-level fields, and a `results` record for every required ID:

The checker resolves evidence paths, including symlinks, and rejects paths inside the checkout. It also inspects the effective Harness module with `go list -m -json`, with module and toolchain downloads disabled. A workspace-main Harness, replacement, resolution error or mismatch with the `go.mod` pin prevents completion. This checks effective dependency identity; the release-provenance review must still establish that the pinned version was actually published.

```json
{
  "schema_version": 1,
  "source_commit": "actual full candidate commit",
  "harness_version": "actual released version from go.mod",
  "plan_sha256": "actual plan digest",
  "manifest_sha256": "actual manifest digest",
  "clean_source": true,
  "native_platforms": ["linux", "darwin"],
  "results": [],
  "metrics": {},
  "review": {"reviewer": "actual reviewer identity", "disposition": "accepted", "report": "review.md"},
  "live_budget_authorised": false,
  "comparison_conclusion": "not_established"
}
```

This is a **non-passing structural example**, not a ready-to-use acceptance report. M0 must produce records from real suite runs. Each `results` item has `id`, `status: passed` and a `scenarios` array whose IDs exactly match the manifest. Each scenario contains:

- `id`, `status: passed`;
- `procedure`: exact test name/command, toolchain, environment, runner identity and replay instructions;
- `expected`: independently specified observable result;
- `observed`: actual outcome, including exit codes and relevant values;
- `artifacts`: nonempty list of `{ "path": "relative/log.jsonl", "sha256": "actual digest" }` inside the evidence bundle.

Raw logs must expose test names, counts, failures and skips. Coverage profiles, per-package/diff reports, screenshots/PTY transcripts, process probes, evaluation raw data, release checksums and review report are all artefacts. Multiple scenarios may legitimately refer to one suite log only when it contains individually identifiable checks for each scenario.

The numeric `metrics` keys enforced by `check.py` are:

- `statement_coverage_pct`, `changed_statement_coverage_pct`;
- `lifecycle_coverage_pct`, `permissions_coverage_pct`, `persistence_coverage_pct`, `execution_coverage_pct`, `checkpoints_coverage_pct`, `budgets_coverage_pct`, `protocol_coverage_pct`, `extensions_coverage_pct`, `accounting_coverage_pct`;
- `required_skips`, `open_acceptance_defects`, `open_p0_p1_defects`;
- `ui_key_p95_ms`, `cancel_start_p95_ms`, `child_cleanup_max_ms`, `critical_repetitions`, `fuzz_seconds_per_target`;
- for full: `live_tasks`, `repeats_per_arm_per_mode`, `live_modes`, `live_arms`, `hand_task_success_pct`, `hand_heldout_success_pct`.

Every metric must reconcile with hashed raw artefacts; do not type convenient values into the report. Zero, unknown and not measured are distinct. The checker rejects missing numeric gates, stale identity, missing/mismatched artefacts, missing scenarios, skips/failures, insufficient coverage, missing platforms/review and omitted live gates for full completion.

### What the checker cannot prove

The validator checks the structure, identity, artefact integrity and declared metrics. It does **not** run the future product suites, parse every kind of log, cryptographically attest a remote CI runner, detect fabricated logs, or judge whether a test oracle is meaningful. A fabricated report can fool any such local checker. Therefore a green checker result alone is not permission to mark the goal complete.

M0 must add actual suite runners and machine extraction of metrics from raw reports. The final acceptance review must independently inspect their commands, counts, raw results and candidate identity. The reviewer may be a human or a separately conducted review; it must not be a copied implementation self-summary. If no independent reviewer is available, record that gate as pending. Do not claim a person or external service approved something without evidence.

The review artefact must include a line-by-line plan traceability table, test-oracle audit, defect list, supported-platform inventory, scope decisions, dependency version/checksum evidence, metric reconciliation and evaluated limitations. Reference it in G-REVIEW's hashed artefacts as well as the top-level review field. No separate user confirmation is required for routine passing automated checks.

## 5. Goal execution and stopping rules

Use this goal text when ready to implement:

> Implement `docs/2026-09-07-hand-implementation-plan.md` in accordance with `docs/acceptance/definition-of-done.md` and all IDs/scenarios in `docs/acceptance/requirements.json`. Deliver each milestone with permanent tests, migration documentation and reproducible evidence. Start with the missing acceptance runners and regression tests. Preserve unrelated user work. Do not weaken requirements, treat skipped/mocked live checks as passes, or use synthetic checker fixtures as product evidence. Record each milestone's implementation and evidence paths. Before completion, freeze a clean candidate, run the required uncached suites on supported platforms, reconcile raw evidence, complete the final acceptance review, and run `python3 docs/acceptance/check.py --profile full --evidence <absolute path to actual run.json>`. Mark the goal complete only when the result is `complete` and its provenance and semantic checks pass. If a gate is pending, continue independent work and report the exact prerequisite. Do not spend on live evaluations without an authorised cap, or publish Hand merely to satisfy the goal. Keep Pi superiority claims separate from implementation completion.

This text does not itself start a goal. It also does not grant missing write access to Harness or approval to publish that dependency; establish those prerequisites when needed without abandoning independent Hand work.

At each milestone, report required/passed/failed/pending scenario counts, new code paths, tests executed, artefact paths and next dependencies. At the end, deliver the candidate SHA, Harness version, coverage report, platform/runtime results, performance figures, live evaluation conclusion, acceptance review and final checker output. If any are missing, the user can see precisely why the goal is not complete.
