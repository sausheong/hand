# Implementation progress

## 8 September 2026 — M0.1 initial infrastructure

Goal remains **in progress**. None of the 32 acceptance requirement groups is closed. The machine-readable [progress record](progress.json) preserves the full backlog and evidence locations.

Implemented:

- Shared PR/branch/reusable CI workflow for macOS and Linux. Tag release publication depends on it for the same source revision.
- Offline runner with pre-run test inventory, raw stdout/stderr logs, uncached race/coverage run, missing/skip/failure detection, build/vet/format checks and credential-free binary smoke tests.
- Runner regression tests for empty/missing selections, skips (including subtests), failures followed by passes, and successful inventory reconciliation.
- Injected version/commit metadata and early `--version` handling. Ordinary Go builds retain available module/VCS metadata.
- Four-target archives include LICENSE; archive verification checks inventory, checksums and native binary startup.
- Removed one pre-existing extra blank line caught by gofmt. No other unrelated source changes.

Evidence:

- `/private/tmp/hand-m0-validation-20260908-01`: correctly failed on existing formatting.
- `/private/tmp/hand-m0-validation-20260908-02`: exposed a runner bug parsing a Go stderr warning as package-list data. Fixed by retaining stderr separately from machine-readable stdout.
- `/private/tmp/hand-m0-validation-20260908-03`: passed build/vet/format, test inventory reconciliation, uncached race/coverage tests, Python verifier/runner tests and credential-free help/version smoke tests.
- `/private/tmp/hand-m0-dist-smoke-20260908.json`: all four target archives passed checksums/content checks; darwin-arm64 executed locally. Other architectures were compiled, not executed.

These are development evidence on a dirty worktree, not a frozen candidate. The binary metadata identifies the base commit while source edits remain uncommitted; no final provenance claim is made. Temporary evidence paths must be archived before a final release audit.

Remaining M0 work includes Harness contract suite organisation, automatic coverage/diff metric extraction and evidence packaging, benchmark/evaluation fixtures, architecture decisions, hosted CI failure-gate evidence and platform runtime qualification. M1–M8 remain pending.

Next: improve candidate/worktree provenance in the runner, finish M0 suite/measurement support, then add the permanent M1 regression tests and fixes. Access to Harness publication, hosted runners and live evaluation budget will be handled when their dependent work is concrete; no release or paid evaluation was performed.

## 8 September 2026 — M0 evidence strengthening and M1.3 permission fixes

The previous turn was progress, not a wait/block. The goal remains in progress.

- M0 runner now records content/mode fingerprints for tracked and untracked source, refuses output inside the checkout, and rejects source changes during validation. Dirty binaries are explicitly labelled. Coverage percentages are computed from raw instrumented blocks, weighted by statements and deduplicated across test binaries. Numeric parser tests reject invalid/contradictory data. Diff coverage, critical-group mappings and complete acceptance report generation remain pending.
- Permanent permission regressions failed against the original implementation: skill mutations were allowed, persistent todo writes bypassed approval, and failed grant persistence installed an in-memory grant. Original failure log: `/private/tmp/hand-m1-permission-before.txt`.
- Added explicit classification of registered tools; unknown tools default to approval. Skill get/list and load remain read-only; create/patch/replace/remove and persistent todos require approval. Both interactive and one-shot paths use the operation classification.
- Grant updates now use Harness's existing atomic-write primitive and commit in-memory state only after persistence succeeds. The currently approved invocation remains allowed on failure, the TUI displays a warning, and the next invocation prompts again. Tests exercise interactive and one-shot semantics and classify the actual registry.
- Updated README to document the intentional permission behaviour change.

Full local validation passed at `/private/tmp/hand-m1-permission-validation-20260908-01/report.json`, including raw race/test events, coverage, Python tests and credential-free binary smoke tests. This remains dirty development evidence, not final acceptance. Progress-file updates made after the run are intentionally outside that recorded snapshot.

Harness checkout was inspected read-only: clean at `09c30fe`, matching the existing v0.3.9-era runtime. It still lacks the last-request usage event required by M1.1. No Harness changes or publication have occurred.

Next available work: M0 contract/benchmark support and M1 search/lifecycle regressions; shared usage/process fixes will need a writable Harness development checkout and eventual released dependency. No milestone or full acceptance group is declared complete yet.

## 8 September 2026 — M1.2 search correctness

The preceding turn was progress. Implemented streaming large-file scanning, rooted filesystem access, explicit result/output caps, cancellation and partial-error reporting, nested gitignore rules and include-ignored/hidden controls. Git metadata/binary/non-regular exclusions are explicit. Added the go-git matcher dependency and documented the choice in ADR 0001. Updated old tests that had required silently ignoring large files to assert the new specified behaviour instead.

Permanent large-file, invalid-glob and cancellation tests failed on the original search implementation (`/private/tmp/hand-m1-search-before.txt`) and passed after replacement. Additional tests cover nested negation, ignored-parent semantics, explicit subdirectory searches, overrides, long lines, bounded output, unreadable files, external symlinks and empty PATH. Unreadable-file qualification runs as an unprivileged user rather than skipping its assertion.

Full local validation passed: `/private/tmp/hand-m1-search-validation-20260908-01/report.json`. This is still dirty development evidence, not a frozen candidate. README heading cleanup and this progress update occurred after the recorded snapshot.

M0.2 gained a search benchmark: twenty 200,000-byte text files, complete no-match scan, Apple M4 Max/macOS ARM64. One run measured 8.51 ms/op, 493.69 MB/s and 200,498 B/op (`/private/tmp/hand-m1-search-benchmark-20260908.txt`). This is a reproducible fixture/baseline, not a comparative advantage or a final performance gate.

Remaining: M0 benchmarks/contracts/evidence aggregation and M1.1/M1.4/M1.5, followed by all later milestones. M1.2 still needs final candidate/platform evidence and review; no full requirement group is closed.

## 8 September 2026 — M1.4 Stop-hook lifecycle regression

The preceding model-selection answer did not advance implementation. Revalidated the current worktree and fixed the next observed lifecycle defect. No acceptance group is closed.

- A permanent regression reproduced a cancelled goal result starting another run; failure evidence is `/private/tmp/hand-m1-lifecycle-before.txt`.
- Stop-hook commands now capture a generation and cancellable context. New user input, session/model changes, compaction and explicit cancellation invalidate the previous check. Consuming a result invalidates duplicates. The footer displays goal-check activity and Ctrl+C cancels that activity.
- Tests exercise actual subprocess results queued before cancellation, newer input and a newer check; verify context cancellation and reject duplicate delivery.
- Full local validation passed at `/private/tmp/hand-m1-lifecycle-validation-20260908-01/report.json`: build, vet, formatting, uncached race/coverage, runner/checker tests and binary smoke. Evidence is from a dirty development snapshot; this progress update follows the snapshot.

Remaining M1.4 work includes session/run identity on stream and approval events, asynchronous cancellable compaction, outcome/exit mappings, process signals, JSON-stdin hook migration and fail-closed mandatory validators. Cancellation here propagates to the existing hook executor; process-tree cleanup still requires M1.5. All broader M0–M8 requirements remain in force.

## 8 September 2026 — M1.4 asynchronous manual compaction

The preceding turn made progress. The current worktree was verified before continuing. A permanent regression demonstrated manual compaction calling the provider synchronously and blocking for its two-second timeout (`/private/tmp/hand-m1-compaction-before.txt`).

Manual compaction now runs as a Tea command with a cancellable context, explicit activity state and generation-tagged result. Ctrl+C cancels the worker; the operation remains busy until that worker returns. New turns/session/model commands cannot race the in-flight mutation. Quit requests cancel and wait for completion. Tests exercise real Harness compaction with a blocked test provider, responsive window updates, cancellation propagation, unchanged session history, cancellation before dispatch, stale result rejection and orderly quit. No live provider calls were made.

Full local validation passed at `/private/tmp/hand-m1-compaction-validation-20260908-01/report.json`, including build/vet/format, uncached race/coverage, runner/checker tests and binary smoke. README and this progress record were updated after the recorded dirty development snapshot. This is not final candidate or native-platform qualification.

M1.4 remains in progress: stream/run/session identity, outcome and exit mappings, hook protocol/failure policies and signal shutdown still need implementation. Harness compaction cancellation relies on provider cooperation; its shared fallback/commit semantics require further review under M1/M6. No requirement group or full acceptance gate is declared complete.

## 8 September 2026 — M1.4 hook input and mandatory validation

Previous turn was progress. Verified the current worktree, then reproduced bulk environment failure on a 1.4 MB prompt and default fail-open validation (`/private/tmp/hand-m1-hooks-before.txt`).

Hooks now receive version 1 JSON stdin with structured tool input and string prompt/output/error fields. Small metadata variables remain available. The explicit legacy environment option is size bounded and documented. Blocking events default to denial on errors; optional hooks may select warning-only behaviour. Configuration loading/saving validates event, policy and timeout, and rejects mandatory policies on observe-only events. Stop validation errors propagate to the one-shot caller and TUI. Cancellation does not fail open.

Capture bounds apply during stdout/stderr writes (64 KiB each). A subprocess regression caught bytes.Buffer's promoted ReadFrom bypassing the bounded Write method; replacing embedding with composition fixed that defect. Pipe draining is bounded; descendant-process termination is still pending shared execution hardening. Permanent tests cover real large stdin payloads, structured inputs, migration, inherited-payload removal, oversized legacy rejection, spawn/timeout/output errors, cancellation, configuration validation, bounded capture and one-shot propagation through a real Harness runtime using a local test provider.

Full local validation passed at `/private/tmp/hand-m1-hooks-validation-20260908-02/report.json`. Earlier `-01` failed because the new test provider lacked its schema-normalisation method; the fixture was repaired and its targeted test passed before fresh full validation. These are dirty development snapshots, not final candidate evidence. This progress update follows the recorded snapshot. No live calls, publication or requirement closure occurred.

Remaining M1.4: stream/session/run identity, exactly-one structured terminal outcome, documented exit-code mapping, signal shutdown and final qualification. Remaining M0/M1 Harness work and M2–M8 retain their original scope.

## 8 September 2026 — M1.4 one-shot outcomes and signals

Previous turn made progress. Verified current CLI and runtime event handling. A permanent regression reproduced goal-iteration exhaustion returning success (`/private/tmp/hand-m1-outcomes-before.txt`).

Added shared RunOutcome/RunFailure types with separate status, reason, iteration count and scoped verification flag. The one-shot driver joins runtime events before classifying completion. It handles max_turns before generic runtime errors, rejects missing completion, distinguishes prompt/Stop validation failure, and returns budget exhaustion when a Stop hook requests more work at the iteration cap. An answer with no mandatory validator remains unverified. Config/invocation errors map to 2, verification 3, limits 4, infrastructure 5 and cancellation 130. SIGINT/SIGTERM cancellation now reaches active one-shot execution. Startup/MCP cancellation and TUI outcome integration remain pending.

Permanent matrix tests use a real Harness runtime and local event providers. They cover plain/verified answers, optional warnings, prompt and Stop failure, both limit types, provider failure and cancellation. Child-process tests send actual SIGINT and SIGTERM to the production signal context/driver and assert exit 130. No external model call was made. README now documents exit semantics and uses a validator example that preserves make's exit status instead of hiding it behind a pipeline.

Full local validation passed at `/private/tmp/hand-m1-outcomes-validation-20260908-01/report.json` (dirty development snapshot). This progress update follows that snapshot. No requirement is closed: TUI session/run/event identity, terminal outcome integration, startup cancellation, platform evidence and all remaining M0–M8 work still apply.

## 8 September 2026 — M1.4 TUI asynchronous identity

Previous turn was progress. Verified current event and approval paths; a permanent regression reproduced EventDone releasing the TUI before runtime channel closure (`/private/tmp/hand-m1-identity-before.txt`).

Added context-carried session/run/generation identity. TUI stream events and channel-close messages, approval requests/warnings, goal checks and compaction results carry identity. The TUI rejects superseded messages and denies stale approvals. EventDone records output/usage but channel closure releases run ownership. Cancellation suppresses late output, dismisses pending approval and waits for the runtime to join; quit also joins. Session/model commands cannot race active runs. The approval hook rechecks cancellation before applying a queued decision, preventing a cancelled always-allow decision from persisting.

Permanent tests cover all three identity components, cancelled output and approvals, duplicate closure, session/model changes after queued goal results, command guards and orderly quit. A cancellation-versus-grant regression passed 20 race-enabled repetitions. Existing interactive tests now use the actual identity captured from Run's context rather than injecting unscoped approval messages.

Full local validation passed at `/private/tmp/hand-m1-identity-validation-20260908-01/report.json`, including uncached race/coverage and binary smoke. Dirty development evidence only; this progress update follows the recorded snapshot. Remaining M1.4 includes TUI structured terminal-outcome integration, startup cancellation and final platform qualification. M0, shared Harness changes and later milestones remain open.

## 8 September 2026 — M1.4 interactive terminal outcomes

Previous turn was progress. A permanent regression reproduced the TUI skipping Stop validation at the iteration cap (`/private/tmp/hand-m1-tui-outcome-before.txt`). The TUI now evaluates that final iteration before deciding completion versus exhaustion, and shares joined-turn failure classification with one-shot mode.

Each user request receives one terminal result across automatic continuations. Completion distinguishes mandatory-check evidence from an unverified answer. Runtime/validation failures, missing completion, cancellation, exhausted limits and superseded requests do not become successful results. Duplicate/stale messages cannot publish a second outcome. Tests cover the outcome matrix and real interactive input/event/check delivery; a two-iteration interaction verifies successful validation at the cap and exactly one final transcript result. Review caught a missing active-request initialization, which was fixed before adding the full interactive regression.

Full local validation passed at `/private/tmp/hand-m1-tui-outcome-validation-20260908-01/report.json`. This is dirty development evidence; the progress update follows its recorded snapshot. No requirement is closed without final qualification. M1.4 still needs startup cancellation tied to shared Harness construction/cleanup and final platform evidence; M0 measurements/acceptance tooling, M1.1/M1.5 shared work and all later milestones remain open.

## 8 September 2026 — M1.5 shared Harness construction and file modes

Previous turn was progress. Original Harness checkout was revalidated clean at `09c30fe2d529d6125f423593f621e970a7ce8735`. Created an isolated clone at `/private/tmp/hand-harness-development-20260908`, branch `codex/hand-hardening`; no original checkout changes or publication occurred.

Added backward-compatible BuildRuntimeContext, propagated MCP handshake/discovery cancellation, staged tool registration until all connections succeed and closed partial connections before returning. MCP shutdown waits are 250 ms per shutdown stage, and client Close is synchronized/idempotent. Tests verify pre-cancel, partial cleanup, no leaked tools, successful connection lifetime independent of construction context, and child reaping. The original hung-handshake regression took 5.21 seconds and failed its bound (`/private/tmp/hand-harness-mcp-before.txt`). The full MCP/runtime suites passed 260 tests/subtests, zero failures/skips (`/private/tmp/hand-harness-construction-tests.jsonl`). An initial sandboxed attempt could not bind localhost; rerunning with approved localhost access resolved that environment restriction.

WriteFileTool now preserves existing ordinary permission bits while retaining 0600 for new files. A permanent regression reproduced losing mode 0751; tests also verify failed replacement leaves content and mode intact. File/tool suites passed 67 tests/subtests, zero failures/skips (`/private/tmp/hand-harness-file-tests.jsonl`). Targeted vet passed.

The staged source diff is preserved in `docs/acceptance/harness-development.patch`; its base, digest and development evidence are recorded in `harness-development.json`. `git apply --check` against the untouched original checkout passed. This patch is not a released Harness version, and Hand still uses v0.3.9. Required follow-up includes shared process-tree/output bounds and usage events, released-version integration, removal of Hand's abandoned-goroutine timeout wrapper, startup cancellation and final native-platform/candidate qualification. No full acceptance group is closed.

## 8 September 2026 — M1.1 shared request usage accounting

Previous turn made progress. Revalidated the staged Harness checkout and inspected normal, fallback, non-streaming retry and compaction paths. Added per-attempt RequestUsage records with request ID/model/category/status and explicit reported/unavailable usage. Runtime emits EventRequestUsage independently of its cumulative EventDone. RunTurn now aggregates refusal retries. Compaction returns records and has a concurrency-safe observer contract for background consumption. Anthropic wire input is normalized to total input including cache; cache counters are subsets. The new USAGE.md documents migration and unknown/billable-failure semantics.

Tests verify ten 20k requests yield 20k latest versus 200k cumulative input, failed and missing usage, fallback model identity, refusal accounting, malformed stream termination, and manual/background compaction records. Initial test failures exposed cancellation being mistaken for empty summary and an early return losing started-tool session pairs. Those paths were fixed; the paired-entry cancellation regression then passed 20 race-enabled repetitions. Fresh full relevant suites passed 386 tests/subtests, zero failures/skips at `/private/tmp/hand-harness-usage-tests.jsonl`, covering llm, runtime, compaction and the local-fixture Anthropic adapter. Targeted vet passed. No live calls occurred.

Refreshed `harness-development.patch` and its digest/manifest, including prior construction/file-mode work. The patch still applies cleanly to the untouched original Harness base. Hand remains on released v0.3.9: its gauge and cache interpretation must be updated together with released-version integration. No claim is made that Hand's M1.1 acceptance passes yet. Shared process-tree/output hardening, remaining acceptance tooling, all later milestones and final platform/candidate evidence remain required.

## 8 September 2026 — M1.5 process groups and bounded capture

The intervening model-choice answer made no implementation progress. This turn revalidated the existing Harness changes and terminal test log, then extended them. The original shell cancellation regression failed because its descendant wrote a delayed marker (`/private/tmp/hand-harness-process-before.txt`). Shared process.Command now creates a Unix process group, kills it on cancellation and bounds inherited-pipe draining; process.Run joins and cleans up descendants after normal exit. Stdio MCP wraps protocol Close with group cleanup. This does not contain children that deliberately detach into new sessions, and non-Unix fallback only kills the direct child.

Added a concurrency-safe bounded capture writer without an embedded buffer, so io.Copy cannot bypass its limit. Bash drains both streams, retains 64 KiB each, reports truncation and observed counts, and preserves cancellation/timeout reasons alongside stderr. Tests verify cancellation and normal-exit descendants cannot write delayed markers, one-megabyte stdout/stderr are bounded with accurate metadata, and concurrent/zero-capacity/copy capture behaviour. Fresh race-enabled process/Bash/MCP/runtime suites passed 294 tests/subtests with zero failures/skips (`/private/tmp/hand-harness-process-tests.jsonl`); targeted vet passed. MCP used local fixtures with approved localhost access, without external model calls.

Refreshed the durable Harness patch and digest; apply-check against the original Harness base passed. Output spooling with disk quotas/retention remains unimplemented; Hand hooks/startup still need adoption after a released Harness version is available. No acceptance group is closed, and these dirty local macOS results are not frozen-candidate or Linux qualification evidence.

## 8 September 2026 — M1.5 bounded output artifacts

Previous turn made progress. Revalidated the staged Harness process/capture changes and the execution requirements before adding disk capture. Shared ArtifactStore/OutputCapture now spill output beyond the memory prefix into private 0600 files. Each artifact is capped at 8 MiB; full-file reservations keep active and retained files within a 64 MiB store allowance. Advisory locks protect active captures across cooperating processes. Completed files are evicted oldest first; allocation also removes completed artifacts older than 24 hours. This is lazy expiry, not a background deletion service. Non-Unix disk capture reports unsupported locking while preserving bounded inline capture.

Bash uses a private per-user temporary store by default, supports an injected store, and returns artifact location, captured byte count, disk truncation and capture errors in metadata and inline notices. Capture failure continues draining without claiming that output was preserved. Artifacts are ephemeral and may be evicted. Tests verify byte-for-byte spill across the prefix boundary, 10 MiB writes capped at 8 MiB on disk, one-megabyte stdout/stderr integration, active quota exhaustion, completed eviction, lazy expiration, private permissions, small-output no-artifact behaviour, allocation-failure draining and reservation enforcement by a separate process.

Fresh race-enabled process/Bash/runtime suites passed 282 tests/subtests, zero failures/skips (`/private/tmp/hand-harness-artifact-tests.jsonl`). Targeted vet passed. The durable Harness patch and digest were refreshed. These are development results, not full acceptance evidence; release integration, Hand hooks/startup adoption, native Linux qualification and all remaining plan milestones still need completion.

## 8 September 2026 — Harness candidate and changed coverage

Previous turn made progress. Full Harness race/coverage suite passed 771 tests/subtests with zero failures/skips, full vet passed, and all four darwin/linux amd64/arm64 targets built. Changed Go files are formatted; unrelated baseline files have pre-existing formatting differences. Cross-build stderr includes nonfatal sandbox module-cache write warnings. Published main and tags were checked read-only: main matches the patch base and latest observed tag is v0.3.9.

Frozen the isolated Harness candidate at `308f702545c138fc5ad51a585c5d6f0a0f6ccc77`, with a native Linux/macOS qualification workflow. Fresh full tests for this clean candidate also passed 771 tests/subtests, zero failures/skips, at `/private/tmp/hand-harness-308f702-tests.jsonl`; its hash and coverage path are in the manifest. Updated the durable diff and prepared `harness-release-proposal.md` for v0.4.0, reflecting cache/input accounting migration. Requested only the missing publication/release authorisation. No branch was pushed, PR opened, merge performed or tag published. Hand still uses v0.3.9.

Independently added changed-statement coverage extraction to Hand's runner. It derives added/changed lines from the recorded baseline, includes untracked production Go files, excludes test files, deduplicates instrumented blocks and counts a complete block when any changed line intersects it. Foreign modules, invalid intervals and conflicting block counts fail; missing profile files remain explicit gaps. Four new runner tests include a real temporary Git checkout/diff. These test fixtures are validation-tool tests, not product evidence.

The first full Hand run `/private/tmp/hand-m0-diff-coverage-validation-20260908-01` measured 77.87% overall and 78.40% changed coverage, below acceptance thresholds. It correctly failed its source-consistency check because a manifest was updated during execution. That bundle is not accepted candidate evidence; a fresh fixed-source run is required. Critical-group mapping and coverage improvement remain open.

The fixed-source rerun `/private/tmp/hand-m0-diff-coverage-validation-20260908-02/report.json` passed its runner checks: 402 Go tests/subtests, zero failures/skips, 14 runner tests and the checker test suite. It recorded 77.93% overall statement coverage and 78.54% changed coverage (549/699 statements), so the 80%/90% acceptance gates remain unmet. This progress entry was added after validation finished. The runner's passed status means its checks executed consistently, not full goal completion or passing acceptance thresholds.

## 8 September 2026 — Cross-package instrumentation and CLI invocation

Previous turn made progress. Inspected low-coverage functions against their tests and found outcome helpers exercised by CLI/TUI tests were omitted from their dependency coverage because the runner used Go's package-local instrumentation default. Added `-coverpkg=./...` to instrument all Hand production packages across test binaries; existing block deduplication prevents double counting. This changes measurement scope to include actual exercised code, without changing thresholds or excluding uncovered files.

Added real invocation tests for positional arguments, negative limits, inappropriate auto-approval, unknown providers, missing credentials, missing proxy URLs and unsupported Gemini URLs. A local HTTP fixture exercises run() through configuration, runtime construction and one-shot completion, and asserts the configured model and exact user prompt arrive at the expected endpoint without duplicate requests. Targeted race tests passed at `/private/tmp/hand-cli-invocation-tests.txt`. No real provider calls were made. Full refreshed validation follows; release authorisation remains pending and no Harness publication occurred.

Fresh full validation passed at `/private/tmp/hand-m0-cross-package-validation-20260908-01/report.json`: 412 Go tests/subtests, zero failures/skips, all runner/checker tests, build/vet/format and smoke checks. Cross-package instrumentation and new invocation coverage measured 85.24% overall (1398/1640) and 85.84% changed statements (600/699). The prior package-local percentages are not directly comparable as test coverage measurements. The overall threshold is met on this development snapshot; changed coverage is below 90%, critical groups remain unqualified, and this does not close any acceptance group. This entry was written after validation finished.

## 8 September 2026 — Critical-subsystem coverage reporting

Previous turn made progress. Added an explicit nine-group source map spanning Hand and Harness, including named pending capabilities for unimplemented work. The reporter deduplicates overlapping mappings and repeated coverage blocks, reports missing module profiles and unmatched patterns, and leaves the acceptance percentage null whenever evidence or implementation is incomplete. Partial observed coverage remains available for diagnosis. It requires all nine groups and rejects conflicting profiles and invalid coordinates. Four new tests verify overlap, missing modules/files/implementation, omitted groups and conflicting block counts; all 18 runner tests passed.

The ordinary validation runner now records critical-coverage.json. A standalone command accepts multiple module profiles and hashes the profiles and mapping, without pretending hashes prove provenance. Development inspection at `/private/tmp/hand-critical-development-20260908.json` combines the prior Hand v0.3.9 test profile with Harness candidate 308f702 solely to locate gaps; it is explicitly not integrated acceptance evidence. All nine groups remain incomplete. Mapping maintenance, percentage semantics and candidate compatibility are documented in coverage-reporting.md. Full refreshed runner validation follows, without changing thresholds or closing requirements.

Full runner validation passed at `/private/tmp/hand-m0-critical-coverage-validation-20260908-01/report.json`: 412 Go tests/subtests, zero failures/skips, 18 runner tests, checker tests and ordinary build/vet/format/smoke checks. The generated report correctly leaves all nine critical groups incomplete. These diagnostic results do not meet the final critical-coverage gate. This progress update was written after the validation snapshot completed.

## 8 September 2026 — Real-terminal startup baseline runner

Previous turn made progress. Added benchmark_startup.py to build and launch the real application in a PTY with fresh HOME/workspace, explicit local model configuration and no submitted model request. It measures launch to rendered typed input after confirming terminal echo is disabled, records raw hashed transcripts, binary identity/size, source/Harness/toolchain/hardware identity and wait4 peak RSS. Cases cover no MCP, fast local MCP, one-second delayed MCP and an unavailable executable. Deadlines, bounded terminal capture and process-group cleanup bound the collector.

One-trial calibration `/private/tmp/hand-startup-probe-20260908-01/report.json` successfully observed all four behaviours: delayed MCP delayed readiness and the unavailable executable prevented TUI launch with exit 5. This is baseline evidence of current behaviour, not optional-MCP acceptance. Three probe tests passed for nearest-rank statistics, timeout/reaping and rejection of terminal echo as a readiness oracle. The procedure and limitations are in startup-benchmark.md. A 30-trial-per-case run follows with source held fixed.

Collected 120 valid observations for the development tree and another 120 for a clean isolated original `e6dd263` checkout, using the same input-readiness procedure. Original p95: no MCP 55.51 ms, fast MCP 68.01 ms, one-second delayed MCP 1,076.63 ms; failed MCP has no readiness measurement. Original binary size is 43,001,010 bytes, with maximum observed process RSS 33,259,520 bytes. Reference hardware is M4 Max/64 GiB. The future identical-configuration no-MCP limit is 116.61 ms under the prescribed formula. Development p95 is 55.60/65.04/1,076.70 ms respectively; no speedup claim is made.

The first 30-trial attempt stopped before measurement because sandboxed sysctl was denied. Approved read-only hardware access resolved that environment restriction. Successful reports and all raw transcripts are preserved in baseline/startup-{original,development}-20260908.tar.gz, with archive digests and verified transcript hashes; compiled binaries are omitted but identified by hash. All 21 runner tests passed after adding the source-checkout option. No acceptance group is closed: later optional-MCP behaviour, task manifest, platform evidence and other M0–M8 work remain required.

## 8 September 2026 — Pinned task-manifest format and oracle calibration

Previous turn made progress. Added evaluation_manifest.py and the versioned calibration catalogue format for repository/agent commits, model/reasoning settings, modes/tools/packages, seeds/repetitions, bounded per-run/total budgets, hashed fixture copies and explicit verification argv/timeouts. Qualification inputs require the full task/held-out/repetition/agent/mode/category dimensions. Draft data remains draft; neither a budget field nor validator success grants permission to execute paid calls. Executor scheduling/scoring and model/source semantic verification remain separate work.

Added a real calibration task for preserving ordinary file modes during write_file replacement. Its hashed Go fixture lives under testdata so it is excluded from Hand production packages. In fresh isolated Harness checkouts, the oracle failed at pinned base 09c30fe on modes 0640/0751 becoming 0600 and passed at candidate 308f702. Raw JSON test logs, stderr, exact commits/commands and digests are preserved under evaluation/oracle-calibration. This proves a known-defect oracle, not an agent task result or comparative advantage.

All 24 runner tests passed, including three new manifest tests for dimensions, immutable pins, duplicate IDs, fixture tampering/path escape and budget constraints. The calibration catalogue correctly returns draft and exits 1 under --qualification. Hand/Pi revision/model/mode entries are intentionally undeclared pending actual evaluation preparation; no approval or missing live evidence was fabricated. The README states limitations and reproduction steps. No full acceptance group is closed; task population, evaluator execution/scoring, release integration and remaining milestones still require completion.

## 8 September 2026 — M2 shared application goal operation

Previous turn made progress. Added internal/app with guarded single-operation ownership, running/checking/cancelling/idle transitions, session/run IDs, ordered sequence numbers, timestamps and immutable value events. Execute owns iteration and mandatory completion checking; Cancel retains ownership until backend/checker join. Completion commits under the cancellation lock so accepted cancellation cannot become success in the final race window. Rejected concurrent operations emit no terminal; each accepted operation emits exactly one. Late cancelled text is suppressed.

Moved the CLI goal loop from main.go to the service and added a Harness translation adapter. The existing CLI outcome matrix and local-provider invocation tests passed after extraction. New tests cover final-iteration verification, event ordering across iterations/runs, backend/checker cancellation joins, concurrent rejection, late cancellation after terminal commitment, malformed/nil streams and missing completion. The service suite passed 20 race-enabled repetitions at `/private/tmp/hand-app-service-repeat-20260908.jsonl`.

The lifecycle decision records the synchronous consumer contract and remaining work honestly: TUI still owns its coordinator, approvals/model/session/compaction have not migrated, bounded subscription delivery is not implemented, durable restart IDs and surfaced persistence errors remain pending. Added app source to critical lifecycle coverage. No M2 acceptance is claimed; full refreshed Hand validation follows against released Harness v0.3.9.

Fresh full validation passed at `/private/tmp/hand-m2-service-validation-20260908-01/report.json`, including 420 Go tests/subtests with zero failures/skips, 24 runner tests, checker tests, build/vet/format and binary smoke checks. The service-specific suite separately passed 160 test/subtest executions across 20 repetitions. These are development results; TUI/shared operation migration remains open and no requirement is marked complete. This progress update follows the completed validation snapshot.

## 8 September 2026 — M2 application-owned model and compaction operations

Previous turn made progress. Moved the TUI controller's runtime mutation logic into app and routed production TUI turns through its ownership gate. Model/session switches and manual compaction reserve the same service gate; cancellation cannot release it until the backend/compaction joins. Background compaction prevents unsafe model/session changes. TUI production code no longer reads or writes Rt fields; session identity is obtained through an accessor.

Provider switches now prepare replacement configuration before committing and never forward a previous custom endpoint across provider boundaries. Failed construction preserves provider/model/endpoint/fallback/identity/compaction configuration. Same-provider changes retain the existing compaction client and fallback. Initial tests caught an accidental same-provider client replacement; fixed that behaviour, then the full app/TUI suites passed at `/private/tmp/hand-app-controller-second.txt`. New tests exercise endpoint isolation, failed-switch atomicity and real runtime/manual-compaction cancellation ownership.

The runtime controller moved files, so critical coverage follows it into internal/app rather than retaining a nonexistent TUI path. The TUI's full goal/check/approval coordinator and bounded event subscription remain unfinished; no M2 acceptance is claimed. Fresh full validation follows the production ownership wiring.

Fresh full validation passed at `/private/tmp/hand-m2-controller-validation-20260908-01/report.json`: 423 Go tests/subtests, zero failures/skips, 24 runner tests, checker tests and build/vet/format/smoke checks. The production TUI construction now supplies the real Harness-backed shared owner. The transitional turn adapter still leaves top-level TUI goal/approval migration unfinished. This evidence update was written after validation completed.

## 8 September 2026 — Bounded application stream and cancellation handles

Previous turn made progress. Added Start with synchronous operation acquisition, bounded progress delivery, independent terminal delivery, run-specific Cancel/Close and a joining Wait that preserves error causes. The CLI now renders this stream. Progress text is split into independent UTF-8-safe chunks no larger than 16 KiB, with 64 queued events. This bounds queued text to 1 MiB plus event overhead, not total backend/request memory. Large error/display metadata is explicitly marked truncated; actual continuation prompts and error causes are retained internally. Image input bytes are copied before asynchronous execution.

Tests cover a full unread queue still permitting cancellation/backend join/terminal delivery, complete Unicode reconstruction and sequence ordering, immediate rejection of overlapping starts, copied image ownership, bounded terminal errors with preserved causes and stale cancellation handles. A new regression caught error truncation splitting UTF-8; fixed. Another caught an old run handle cancelling a later compaction reservation because run IDs alone did not identify all operations; fixed using a separate operation generation. Both failing logs are retained. The corrected app suite passed 20 race-enabled repetitions (340 test/subtest executions) at `/private/tmp/hand-app-stream-final-repeat-20260908.jsonl`. Added an early cancellation check during text chunking before full validation.

The lifecycle decision documents delivery bounds and consumer responsibilities. TUI goal/tool/approval migration, persistence error propagation and remaining plan work are still open. No acceptance group is closed. Full refreshed Hand validation follows.

Fresh full validation passed at `/private/tmp/hand-m2-stream-validation-20260908-01/report.json`: 429 Go tests/subtests, zero failures/skips, 24 runner tests, checker tests and build/vet/format/smoke checks. Overall coverage is 86.24%; changed coverage is 87.53%, still below the required 90%. After the final cancellation-aware chunking change, the full-queue cancellation regression separately passed 20 fresh race-enabled repetitions at `/private/tmp/hand-app-stream-cancel-final-20260908.jsonl`. This progress update follows completed validation and repeats; no full acceptance group is closed.

## 8 September 2026 — Application tool, usage and compaction events

The intervening model-selection answer made no implementation progress. Revalidated the worktree and resumed M2. Added immutable application display snapshots for tool calls/results, usage presence/counters and compaction. Runtime aborts now produce an explicit failure. Variable-size details share an aggregate 16 KiB cap with explicit truncation and independent string storage; display identifiers cannot be used as approval capabilities. Documented queue bounds separately from temporary metadata serialization/backend allocations and retained released Harness usage semantics.

Permanent tests verify copied JSON/metadata survives source mutation, missing usage differs from reported zero, compaction details survive translation, malformed events fail explicitly, oversized details preserve Unicode, and the shared goal stream delivers tool/result/text/usage in order before completion. The full app suite passed 20 race-enabled repetitions (440 test/subtest executions, zero fail/skip) at `/private/tmp/hand-app-details-repeat-20260908.jsonl`. App/TUI/CLI suites passed at `/private/tmp/hand-app-details-verified-20260908.txt`. The initial sandboxed CLI attempt failed to bind its local HTTP fixture; reran with approved listener access, without changing tests. Full validation follows; TUI goal/approval migration remains open and no requirement is closed.

Fresh full validation passed at `/private/tmp/hand-m2-details-validation-20260908-01/report.json`: 434 Go tests/subtests, zero failures/skips, runner/checker tests and build/vet/format/smoke checks. Changed coverage is 88.22%, still below the required 90%. The source remained fixed throughout validation; this evidence update follows its completion. No full acceptance group is closed.

## 8 September 2026 — Production TUI shared goal stream

Previous goal turn made progress. Production TUI now executes the same application Service.Start operation as CLI. It pulls ordered immutable events through bounded delivery, renders tools/usage/continuations, uses the service identity for approval matching and receives the original terminal outcome after backend/checker join. The TUI remains busy during completion checks; cancellation cannot release configuration guards early. Program closure cancels and joins its active goal. Kept the legacy Runner adapter for existing internal embedders/tests during migration; approval state/protocol still needs application ownership.

New tests exercise two-iteration final verification without duplicate checks, tool/usage rendering, service identity propagation, a real approval hook decision through the UI, cancellation with a deliberately held checker, stale event rejection and cleanup of an unread stream after closure. App/TUI suites passed at `/private/tmp/hand-tui-application-second-20260908.txt`; all four new application TUI journeys passed 20 race-enabled repetitions (80 executions, zero failures/skips) at `/private/tmp/hand-tui-application-repeat-20260908.jsonl`. Full validation follows. No full acceptance group is closed.

Fresh full validation passed at `/private/tmp/hand-m2-tui-validation-20260908-01/report.json`: 438 Go tests/subtests, zero failures/skips, runner/checker tests and build/vet/format/smoke checks. Overall coverage is 86.46%; changed coverage is 87.75%, still below the required 90%. This progress update follows completion of the unchanged-source validation run. No full acceptance group is closed.

## 8 September 2026 — Application-owned interactive approvals

Previous goal turn made progress. Added application-owned approval IDs, a bounded pending registry, AwaitingApproval transitions and once-only RespondApproval with full run identity/context checks. Interactive production hooks now transport requests, resolutions and persistence warnings through the application stream; the TUI responds through the service and main no longer needs programSender. Classification/persistence reuse agentio. Cancelled, stale, duplicate and invalid decisions cannot authorise the operation; requests are removed before backend completion. Display truncation never changes the operation being authorised.

Tests exercise service state/identity/decision validation, cancellation, registry cleanup, rejection outside an owned run, and actual TUI approve/cancel journeys without legacy response channels. Review corrected resolved-event ordering to leave AwaitingApproval before publication. The final regression set passed 20 race-enabled repetitions (140 test/subtest executions, zero failures/skips) at `/private/tmp/hand-app-owned-approval-final-repeat-20260908.jsonl`. Earlier app/TUI suites passed at `/private/tmp/hand-app-approval-second-20260908.txt`. Full validation follows. Persistence errors, legacy migration and remaining acceptance work still prevent closing M2.1.

Fresh full validation passed at `/private/tmp/hand-m2-approval-validation-20260908-01/report.json`: 445 Go tests/subtests, zero failures/skips, runner/checker tests and build/vet/format/smoke checks. Overall coverage is 86.34%; changed coverage is 87.25%, still below the required 90%. This progress update follows completion of the unchanged-source validation run. No full acceptance group is closed.

## 8 September 2026 — Named model profiles and resolution

Previous goal turn made progress. Added validated profiles with provider/model/endpoint/credential-reference and capability fields, default_profile and startup --profile selection. Legacy model/base_url resolves equivalently in memory without rewriting existing files. Named profiles never inherit legacy endpoints or conventional provider credentials. Endpoint URLs reject embedded credentials/query/fragment; unsupported local authentication is explicit. Startup applies declared reasoning and explicit context to runtime/TUI. Legacy model switching clears stale context/reasoning overrides when selecting a different model.

Added resolver precedence for explicit limits, verified server metadata, versioned catalogue metadata and labelled fallback. Local advertised maxima cannot substitute for active context. Tests cover migration/round-trip, profile copy isolation, unknown/invalid settings, reasoning capabilities, metadata precedence and a real CLI request proving the named proxy receives only its referenced credential despite another provider key being present. Affected config/app/TUI/CLI suites passed at `/private/tmp/hand-profile-integration-20260908.txt`. Added final validation for unsupported local authentication and tiny context budgets before full validation.

The development interface and remaining gaps are documented in model-profiles.md. Runtime still uses automatic context detection without an explicit override; live discovery/catalogue, model picker/switching, input enforcement and serving-model provenance remain unfinished. Installed Harness fixes output at 8192, so other explicit max_output values fail actionably rather than being silently ignored. No M2.2 scenarios are closed; full validation follows.

Fresh full validation passed at `/private/tmp/hand-m2-profiles-validation-20260908-01/report.json`: 450 Go tests/subtests, zero failures/skips, runner/checker tests and build/vet/format/smoke checks. Overall coverage is 85.76%; changed coverage is 86.47%, still below the required 90%. This progress update follows completion of the unchanged-source validation run. No full acceptance group is closed.

## 8 September 2026 — Atomic profile switches and settings flags

Previous goal turn made progress. Added private copied profile choices in the application controller, guarded SwitchProfile and TUI /profile listing/switching. Every switch constructs a new client, including same-provider profile changes, before committing provider/model/endpoint/context/reasoning/fallback and compaction configuration. Failed construction changes nothing. Same-provider fallback remains intact; crossing providers clears it. UI obtains the resolved runtime context after switching. Ordinary model switches clear the named profile label.

Tests prove failed-switch rollback, local-to-hosted-to-proxy configuration routing with distinct credential references, defensive profile copies, consistent compaction/model/context state, invalid/busy rejection before construction and TUI/runtime agreement. These passed 20 race-enabled repetitions (60 test executions, zero failures/skips) at `/private/tmp/hand-profile-switch-repeat-20260908.jsonl`. Added --context-limit and --reasoning flags plus invalid/unsupported invocation cases before full validation. Metadata discovery, richer picker, input enforcement, provenance and Harness output configuration remain open; no M2.2 scenario is closed by these development tests alone.

Fresh full validation passed at `/private/tmp/hand-m2-profile-switch-validation-20260908-01/report.json`: 456 Go tests/subtests, zero failures/skips, runner/checker tests and build/vet/format/smoke checks. Overall coverage is 85.95%; changed coverage is 86.71%, still below the required 90%. This progress update follows completion of the unchanged-source validation run. No full acceptance group is closed.

## 8 September 2026 — Profile input admission and runtime context agreement

Previous goal turn made progress. Application Start and Execute now enforce declared input types under the operation lock before assigning a run ID or invoking the backend. Declarations are copied, capability updates reject active operations and profile switches update admission with the other runtime fields. Startup configures the shared service for declared capabilities; absent declarations preserve legacy behaviour rather than claiming verified image support.

Found and fixed a fallback mismatch: the UI/controller passed a provider-prefixed local name into context lookup while released Harness evaluates its bare runtime model. The UI now reads the runtime calculation after startup and model/profile switches. This is consistency with the installed runtime, not verified local active-context discovery; that metadata work remains open.

Three new regressions cover rejected image input before backend execution, both entry points, copied declarations, busy reconfiguration, profile switching and actual runtime fallback calculation. They passed 20 race-enabled repetitions (60 executions, zero fail/skip) at `/private/tmp/hand-profile-input-repeat-20260908.jsonl`. Added an invalid capability-declaration case before full validation. No acceptance scenario is closed; full validation follows.

Fresh full validation passed at `/private/tmp/hand-m2-input-validation-20260908-01/report.json`: 459 Go tests/subtests, zero failures/skips, runner/checker tests and build/vet/format/smoke checks. Overall coverage is 86.04%; changed coverage is 86.83%, still below the required 90%. This progress update follows completion of the unchanged-source validation run. No full acceptance group is closed.

## 8 September 2026 — Bounded server metadata discovery

Previous goal turn made progress. Added opt-in llama.cpp properties discovery against an explicit same-origin metadata URL, following the upstream server contract. Requests use the selected profile credential only, reject redirects, cap the body at 1 MiB and time out after two seconds. Invalid/router/advertised-only properties fail explicitly. Decoded metadata records its source URL and response digest; default_generation_settings.n_ctx supplies active context unless an explicit override exists. Startup and profile switching now apply this value. This is server-reported configuration, not independent serving-model proof.

Moved profile switching off the Bubble Tea update loop to avoid blocking keyboard handling during discovery. Cancellation is checked before commit; terminal closure joins the switch worker, and generation-tagged stale results are ignored. Tests cover active-versus-advertised values, explicit precedence, selected credentials, origin constraints, malformed/oversized/redirect/error responses, request cancellation, asynchronous UI cancellation and preserved runtime configuration. Affected suites passed at `/private/tmp/hand-metadata-verified-20260908.txt`; final metadata/cancellation regressions passed 20 race-enabled repetitions (200 test/subtest executions, zero failures/skips) at `/private/tmp/hand-metadata-repeat-20260908.jsonl`.

Other metadata protocols, durable provenance, versioned catalogue and actual serving-model identification remain unfinished. No acceptance group is closed. Full validation follows with source held fixed.

Fresh full validation passed at `/private/tmp/hand-m2-metadata-validation-20260908-01/report.json`: 469 Go tests/subtests, zero failures/skips, runner/checker tests and build/vet/format/smoke checks. Overall coverage is 85.90%; changed coverage is 86.58%, still below the required 90%. This progress update follows completion of the unchanged-source validation run. No full acceptance group is closed.

## 8 September 2026 — Requested model and context provenance in events

Previous goal turn made progress. Preserved prepared profile names and context sources into application ModelInfo values on every event. Source labels distinguish explicit overrides, server properties with response digests and runtime heuristics. Profile/model switches update future events while prior snapshots remain unchanged. Serving-model identity remains unknown rather than reusing a requested aggregator alias. ModelInfo strings have a separate 16 KiB aggregate bound with explicit truncation; updated the documented conservative queue bound.

TUI /usage now displays the last recorded requested model/profile, context source and unknown serving identity. Three application regressions passed 20 race-enabled repetitions (60 executions, zero failures/skips) at `/private/tmp/hand-model-info-repeat-20260908.jsonl`; added a UI rendering regression before full validation. This is in-memory provenance, not durable evidence or proof of the actual serving model. Catalogue, durable persistence, request-level serving identity and remaining acceptance work remain open; full validation follows.

Fresh full validation passed at `/private/tmp/hand-m2-model-info-validation-20260908-01/report.json`: 473 Go tests/subtests, zero failures/skips, runner/checker tests and build/vet/format/smoke checks. Overall coverage is 85.99%; changed coverage is 86.69%, still below the required 90%. This progress update follows completion of the unchanged-source validation run. No full acceptance group is closed.

## 8 September 2026 — Reusable persistence-error follow-on

Previous goal turn made progress. Inspection confirmed that released Harness logs store errors without exposing them to Hand. Created a separate local checkout based on frozen candidate 308f702, preserving that candidate and original Harness checkout. Added per-session sticky PersistenceError, file/directory Flush with surfaced errors, primary runtime prompt/answer failure checks, and RunSync draining through cleanup after errors. Later successful writes cannot erase lost-entry state; unrelated sessions do not inherit another session's error.

Local follow-on commit ec89b4d718182ddde190c036907522a166f7e7d6 passed fresh full Harness race/coverage validation: 778 tests/subtests, zero failures/skips. Final targeted persistence and RunSync regressions passed 20 repetitions (140 executions); session/runtime vet passed. A reconstructable incremental patch and hashed evidence manifest are stored as harness-persistence.patch/json, with scope and limitations in harness-persistence-proposal.md.

Hand remains on released v0.3.9 and has not integrated this API. The follow-on is not published and is not covered implicitly by the earlier candidate's pending release request. Deferred/background persistence paths, atomic rewrites, recovery, leases, catalogue/migration and the rest of M3 remain open. No full acceptance group is closed; Hand source was not changed this turn, so the previous Hand validation remains the last applicable product test run.

## 8 September 2026 — Atomic rewrites and compaction ordering

Previous goal turn made progress. Reproduced the existing rewrite data-loss defect: a later JSON encoding error leaves a rewritten prefix after the original file was truncated. Preserved the failing log. The local Harness follow-on now writes and synchronizes a private same-directory temporary file before rename, then synchronizes the directory. Pre-rename errors preserve the original; post-rename durability errors remain explicit degraded state. Ordinary failure paths remove the temporary file.

Aligned rewrite snapshot locking with Append and protected the legacy compaction rebuild. Appends remain ordered behind the replacement snapshot. The full session suite passed 20 race-enabled repetitions (700 tests/subtests), including concurrent rewrite/append and compaction/append reload comparisons. Local commit c6c155d314f176527d0dd3e33253d592bf82ef61 then passed fresh full Harness race/coverage validation: 782 tests/subtests, zero failures/skips; session/runtime vet passed.

Stored the incremental patch, evidence hashes and remaining scope in harness-rewrite.patch/json and harness-rewrite-proposal.md. No publication or Hand dependency change occurred. Process-kill recovery, stale-temp handling, leases, full DAG/selected-leaf semantics, migration and deferred/background persistence remain open. No full acceptance group is closed; Hand source was unchanged this turn.

## 8 September 2026 — Exclusive session writer leases

Previous goal turn made progress. Added LoadExclusive with a lifetime kernel lease and Session.Close joining synchronous writes before release. Lease identity is fixed to store/agent/key; stale PID metadata never overrides an active lock. Updated store mutations take operation leases when they do not already own the lifetime lease, so append/rewrite/sync/create/delete/rename cannot bypass an exclusive owner. Linux/macOS use nonblocking flock and close-on-exec/no-follow file opening; unsupported platforms reject exclusive mode while preserving legacy operations.

Permanent regressions cover competing writers, protected deletion/renaming, legacy append rejection, changed-key rejection, closed-writer behaviour and an actual child-process death followed by successful lease reacquisition. The session suite passed 20 race-enabled repetitions (780 tests/subtests). Local commit dfae3db0f2f016089e4be29f14d97bee40f24e4b then passed fresh full Harness race/coverage validation: 786 tests/subtests, zero failures/skips. Session/runtime vet passed. Session test binaries compiled for Linux arm64/amd64 and macOS amd64, without claiming native execution there.

Stored the incremental patch and evidence manifest/proposal as harness-lease.patch/json and harness-lease-proposal.md. Truncated-record recovery, catalogue/migration, selected branch persistence, complete durable outcomes and Hand integration remain open. No publication occurred, Hand source was unchanged and no full acceptance group is closed.

## 8 September 2026 — Strict session record loading

Previous goal turn made progress. Replaced silent malformed-record skipping with bounded parsing and RecordError locations. Only an unfinished JSON object at physical EOF is classified as a potential recovery tail; interior/newline-terminated corruption, invalid UTF-8, non-object records, trailing JSON and oversized records fail without modifying the source file. Complete final records without a newline remain valid, and Append now inserts their missing delimiter before writing another entry.

The session suite passed 20 race-enabled repetitions (980 tests/subtests). FuzzSessionRecords passed its 60-second requested run in 61.01 seconds with 3,395,073 executions. Local commit ce578590c0e38b3e204bc10b17fb995759449441 then passed fresh full Harness race/coverage validation: 802 tests/subtests, zero failures/skips; session/runtime vet passed. Stored the incremental patch and hashed evidence as harness-records.patch/json with limitations in harness-records-proposal.md.

The next recovery step must preserve the original backup and remove only a verified incomplete tail while holding the writer lease. Complete DAG/schema validation, version handling, catalogue/migration and Hand integration are still open. No publication or Hand source change occurred; no full acceptance group is closed.

## 8 September 2026 — Backed-up incomplete-tail recovery

The preceding model-selection answer was advisory and made no implementation progress. This goal turn resumed local implementation. Added leased LoadRecovering with durable complete-original backup, exact-byte-prefix atomic replacement, cancellation, explicit repair reporting and retained backup on post-replacement failure. Normal Load remains non-mutating.

Permanent tests cover backup contents, unknown-field preservation, private modes, repeat recovery, appending after repair, corruption rejection, active writers, symlinks, empty-prefix repair, sync failures and cancellation after backup. Two real subprocess kill boundaries verify restart and retained backups. Local Harness commit 46dfa7b3f20b3e16189610c7a7ef44276c102689 passed 821 full race/coverage tests/subtests with zero failures/skips and 1480 session tests/subtests across 20 race-enabled repetitions; session/runtime vet passed. Stored the incremental patch, proposal and hashed raw evidence as harness-recovery.patch/json and harness-recovery-proposal.md.

Hand still uses released Harness v0.3.9. No publication occurred and no full acceptance group is closed. Complete schema/DAG validation, catalogue/migration, selected-leaf persistence, Hand integration and native-platform qualification remain open.

## 8 September 2026 — Durable branch selection and preserved compaction graphs

Previous goal turn made progress on backed-up recovery. This turn added synced selection control records and restart-aware loading; legacy compaction retains original branches and uses fresh message IDs. Strict graph validation exposed automatic compaction's duplicate-ID overwrite; CommitCompaction now atomically installs summary and preserved clones and rejects stale leaf snapshots. Invalid graph records fail without tail repair.

Two permanent regressions fail against base 46dfa7b3f20b3e16189610c7a7ef44276c102689 and pass after the changes. Local candidate 44309157ae4563839a19cf60bd57492cbbd3cbce passed 838 full race/coverage tests/subtests, zero failures/skips; session/compaction suites passed 3,040 tests/subtests across 20 repetitions. Parser fuzzing passed 60 requested seconds (61.04 elapsed; 423,691 executions), and vet passed. Incremental patch, proposal, counterfactual and raw evidence hashes are in harness-branches.patch/json and harness-branches-proposal.md.

Old automatic-compaction logs may contain duplicate IDs and require a backed-up versioned importer before integration. Session catalogue, full schema/version validation, attachment/export limits and native/platform acceptance remain open. Hand remains on released Harness v0.3.9, no publication occurred and no full acceptance group is closed.

## 8 September 2026 — Backed-up legacy compaction migration

Previous goal turn made progress on persisted branches and exposed duplicate IDs in historical compaction logs. Added deterministic ConvertLegacySession and leased MigrateLegacy. They retain original graph versions and unknown fields, reject unrelated corruption, preserve compaction range provenance and install converted logs only after a durable private original backup. Valid/repeated migration does not rewrite the log.

Local candidate 4c78cf45c5439e34f7b2789d1d1c9dc85ddab618 passed 862 full race/coverage tests/subtests with zero failures/skips, 2300 session tests/subtests across 20 race-enabled repetitions, and 60-second converter fuzzing (61.02 elapsed, 4,395,062 executions). File/subprocess regressions cover backup equality, idempotence, writer exclusion, selected-branch restart, sync failure, cancellation and process death at both durability boundaries. Vet passed. Incremental patch, proposal and raw evidence hashes are in harness-migration.patch/json and harness-migration-proposal.md.

Workspace/session catalogue migration, full schema/version handling, durable session identity, attachment/export limits and released integration remain pending. Hand still uses Harness v0.3.9. No publication occurred and no full acceptance group is closed.

## 8 September 2026 — Versioned durable session identities

Previous goal turn made progress on legacy-log migration. Added first-record schema-versioned identity headers, stable across creation/append/rewrite/rename and excluded from conversation history. New creation commits a synced complete file without overwriting an existing key. Migration now backs up and upgrades headerless valid/empty logs, reporting FormatUpgraded; future/invalid headers are rejected by loading, recovery and migration.

Local candidate 5cc93a3d6a1a504cd348d435db8ebd7e68050d28 passed 875 full race/coverage tests/subtests with zero failures/skips and 2560 session tests/subtests across 20 race-enabled repetitions. Both record and converter fuzz targets passed 60 requested seconds; durations/counts and raw hashes are in harness-format.json. Vet passed. Stored the incremental patch/proposal and committed the Harness SESSION_FORMAT.md specification.

Workspace catalogue and product session flows, payload-specific schemas, attachment/export limits, released integration and native/platform acceptance remain pending. Hand still uses Harness v0.3.9. No publication occurred and no full acceptance group is closed.

## 8 September 2026 — Workspace catalogue groundwork

Previous goal turn made progress on durable Harness identities. Added Hand Catalogue for canonical workspaces, multiple durable backend bindings, session names and last-active selection. Locked atomic updates preserve earlier records; registration is idempotent and invalid metadata is rejected. Permanent real-file and process-death regressions passed 260 tests/subtests across 20 race-enabled sessionio repetitions.

The first full runner attempt rejected an uncompiled fallback for unadvertised platforms. Removed that unnecessary new file and collected fresh validation without changing coverage rules. /private/tmp/hand-m3-catalogue-validation-20260908-02/report.json passed 482 tests/subtests with zero failures/skips, build/vet/format checks and binary smoke. Overall coverage is 85.78%; changed coverage is 86.32%, still below 90%. Stored hashed evidence in catalogue-development.json and implementation/remaining integration notes in session-catalogue.md.

The catalogue is not wired into main.go or Controller.NewSession yet; current product new-session paths remain destructive until that integration. Backend creation/registration reconciliation, legacy migration, new/resume/name/fork/tree/export flows, released Harness integration and native-platform acceptance remain open. No full acceptance group is closed.

## 8 September 2026 — Catalogue durability retries and commit interruption

Previous goal turn made progress on workspace catalogue groundwork. Fixed false success on idempotent registration after an earlier post-rename directory-sync failure: retries now sync the existing index and directory. Added file-sync/rename/directory-sync fault tests, size/session-count/generation bounds and real subprocess termination before/after rename, checking complete state and nonduplicating restart.

Fresh full Hand runner passed 492 tests/subtests with zero failures/skips, build/vet/format and binary smoke checks. Sessionio passed 460 tests/subtests across 20 race-enabled repetitions. Overall coverage is 85.95%; changed coverage is 86.57%, below the 90% requirement. Cross-compiled sessionio test binaries for Linux amd64/arm64 and macOS amd64 without claiming native execution. Evidence is in catalogue-durability.json and /private/tmp/hand-m3-catalogue-durability-validation-20260908-01/report.json.

Requested updated Harness publication permission for candidate 5cc93a3, covering a PR and gated v0.4.0 release after native CI/review, excluding Hand publication and paid evaluations. Prepared the full candidate patch and review scope in harness-current-release.patch/json/md. Permission remains pending; no publication occurred. Product session integration, backend/catalogue reconciliation and full acceptance remain open.

## 8 September 2026 — Isolated real-Harness session integration

Previous goal turn made progress on catalogue durability. While current Harness release permission remains pending, copied Hand into /private/tmp/hand-session-integration-20260908 and used a temporary local replacement for Harness 5cc93a3 only in that checkout. Implemented workspace Manager reconciliation/migration, lease-backed new/resume, backend identity validation, startup selection, --session, non-destructive --new-session and controller/TUI /new wiring there. Primary dependency/code remain unchanged by this staged integration.

The isolated full race/coverage suite passed 501 tests/subtests, zero failures/skips, and sessionio passed 580 tests/subtests across 20 race-enabled repetitions; vet passed. A built-binary test ran three separate processes against a local HTTP fixture and verified independent new-session context, restored original context and retention of both sessions. The initial local-port sandbox failure is retained; final tests ran with the required permission. Saved the reconstructable integration patch, source hashes, dependency provenance and raw evidence as session-integration.patch/json/md. The temporary go.mod replacement is excluded from the patch and is not final qualification evidence.

Release/native gates, background-compaction shutdown joins, remaining interactive resume/name/fork/tree/export flows, usage/attachments and full acceptance remain open. No publication occurred and no full requirement group is closed.

## 8 September 2026 — Staged interactive resume and naming

Previous goal turn made progress on isolated real-Harness session integration and binary new/resume evidence. Added guarded controller resume/name/list APIs and /resume, /name TUI commands, sanitised selected-history replay, stale-event generation changes and clearing of prior-session transient counters. Failed/busy targets preserve the current writer/view; same-session resume does not reacquire its own lease.

The staged full race/coverage suite passed 504 tests/subtests with zero failures/skips, and four controller/TUI regressions passed 20 repetitions (80 tests/subtests); vet passed. Preserved prior artifacts and wrote the new aggregate session-ui-integration.patch plus source/evidence hashes and scope in session-ui-integration.json/md. The patch excludes the isolated local module replacement and passes application checks against the primary source.

Primary Hand remains unchanged by this staged integration and uses Harness v0.3.9. Release approval remains pending. Interactive PTY qualification, asynchronous large-session responsiveness, persisted usage/attachments, fork/tree/export and shutdown/lifecycle completion remain open. No full requirement group is closed.

## Asynchronous session selection development evidence

Staged `/new` and `/resume <ID>` now move selection/replay to a cancellable worker with generation checks and shutdown joining. Cancellation retains the command guard until completion; late cancellation preserves the committed backend identity. Fixed the existing queued-goal test to join its new worker before temporary-directory cleanup while preserving its immediate invalidation assertion.

Fresh staged full race/coverage validation passed 508 tests/subtests with zero failures/skips; targeted switching and queued-goal checks passed 20 repetitions (140 tests/subtests), and vet passed. The aggregate [patch](session-async-integration.patch), [scope and limitations](session-async-integration.md), and [source/raw evidence hashes](session-async-integration.json) are preserved separately from previous snapshots. Primary integration and released Harness qualification remain pending. All M3.1 scenario obligations remain open.

## Asynchronous session metadata development evidence

Session listing and naming now run through the staged session worker, preserving identity, transcript and counters. Added permanent view-preservation and cancelled-context/no-mutation assertions. The full race suite passed 512 tests/subtests without failures/skips; targeted session checks passed 20 repetitions (200 tests/subtests), and vet passed. See [scope](session-metadata-integration.md), [aggregate patch](session-metadata-integration.patch), and [hashed evidence](session-metadata-integration.json). Primary integration is unchanged and all M3.1 scenario obligations remain open.

## Session tree navigation development evidence

Staged `/tree` lists topology and selection; `/tree <entry ID>` persists branch selection and replays its history while preserving other branches. Real-file regressions verify selected-leaf restart, original-branch retention, control exclusion and invalid/cancelled/active-run guards. The fresh full race suite passed 514 tests/subtests, two tests passed 20 repetitions (40 passes), and vet passed. See [scope](session-tree-integration.md), [aggregate patch](session-tree-integration.patch), and [hashed evidence](session-tree-integration.json). Primary released-dependency integration and all M3.1 full scenario obligations remain pending.

## Session fork development evidence

Staged `/fork` creates a separate selected-history session through a private durable staging copy and atomic publication before catalogue registration. It preserves the source graph and selected leaf. Full race validation passed 516 tests/subtests; two fork regressions passed 20 repetitions (40 passes), and vet passed. See [scope and pending fault qualification](session-fork-integration.md), [aggregate patch](session-fork-integration.patch), and [hashed evidence](session-fork-integration.json). Primary integration and all full M3.1 scenario obligations remain pending.

## Fork interruption development evidence

Added deterministic real-process kill checks at four fork publication boundaries. Twenty repetitions yielded 80 killed-child trials; a separate four-boundary injected-error suite passed 80 trials. Both preserve source selection and recover only complete published forks. Full staged race validation passed 527 tests/subtests without failures/skips, and vet passed. See [scope and limits](session-fork-crash-integration.md), [aggregate patch](session-fork-crash-integration.patch), and [raw evidence hashes](session-fork-crash-integration.json). No full M3.1 scenario closure or released-dependency qualification is claimed.

## Fork staging cleanup development evidence

Added owner-locked abandoned-staging cleanup with bounded directory scanning/removal and live-copy/symlink protection. Fixed the fresh-install missing-store regression found by the full suite and added a permanent test. Fresh full race validation passed 530 tests/subtests; cleanup/process-death checks passed 20 repetitions (160 events), and vet passed. See [scope](session-fork-staging-integration.md), [aggregate patch](session-fork-staging-integration.patch), and [hashed evidence](session-fork-staging-integration.json). Full acceptance remains pending.

## JSONL export development evidence

Staged `/export <new path>` writes complete session JSONL through a private synced temporary file and exclusive publication, preserving graph/selection and refusing overwrite. Enforced total/per-record limits and added round-trip, spaced-path, permissions, boundary, cancellation and short-write tests. Full race validation passed 537 tests/subtests; export tests passed 20 repetitions (140 events), and vet passed. See [scope](session-export-integration.md), [aggregate patch](session-export-integration.patch), and [hashed evidence](session-export-integration.json). Full M3.1 and released-dependency acceptance remain pending.

## Offline CLI export development evidence

Added `--session ID --export-session PATH` before model/trust/runtime setup, with conflicting-flag validation and signal cancellation. A compiled-binary journey verifies byte-identical export without valid model config or credentials, preserving active session selection and refusing overwrite. Full race validation passed 541 tests/subtests without failures/skips; vet passed. See [scope](session-cli-export-integration.md), [aggregate patch](session-cli-export-integration.patch), and [hashed evidence](session-cli-export-integration.json). Full acceptance remains pending.

## JSONL export interruption evidence

Added real child-process termination before/after export publication and error-return cleanup tests. Forty killed-child trials passed over 20 repetitions; destination absence/completeness, unchanged source bytes and released writer ownership are asserted. Fresh full race validation passed 548 tests/subtests without failures/skips, and vet passed. See [scope](session-export-crash-integration.md), [aggregate patch](session-export-crash-integration.patch), and [hashed evidence](session-export-crash-integration.json). Full acceptance remains pending.

## Real session write-failure development evidence

Added child-local RLIMIT_FSIZE tests that exercise actual EFBIG writes during fork/export. Forty trials passed over 20 repetitions, verifying no partial publication, unchanged source/catalogue, writer release and retry after restoring the limit. Full race validation passed 552 tests/subtests without failures/skips; vet passed. See [scope](session-write-fault-integration.md), [aggregate patch](session-write-fault-integration.patch), and [hashed evidence](session-write-fault-integration.json). Explicit ENOSPC/permission and full acceptance gates remain pending.

## Durable metadata foundation

Implemented Harness annotation controls at local unpublished c16dccd: durable application metadata does not move the conversation leaf or enter model history, and survives rewrites. Updated the staged Hand tree filter. Harness full race validation passed 878 tests/subtests; annotation regressions passed 20 repetitions; parser fuzzing passed after 61.480 seconds. The Hand full suite passed 553 tests/subtests after separating it from competing fuzz workers; the initial viewport timing failure is preserved. Both vet runs passed. See [scope and release implications](session-annotation-integration.md), [Hand patch](session-annotation-integration.patch), [Harness patch](harness-annotations.patch), and their JSON evidence manifests. Usage recording/restoration itself is still pending.

## Persisted provider-attempt usage

Connected Harness usage observation to durable versioned annotations, added validated/deduplicated whole-session totals and unknown-attempt counts, and restored accounting at startup/resume. Backend completion emits a session snapshot and persistence failures. Full staged race validation passed 560 tests/subtests; usage checks passed 20 repetitions (140 events), and vet passed. See [implemented scope and lifecycle gaps](session-usage-integration.md), [aggregate patch](session-usage-integration.patch), and [hashed evidence](session-usage-integration.json). Background/manual compaction, cancelled-view refresh, legacy uncertainty and full M3.1 acceptance remain pending.

## Cancelled-run terminal accounting

Captured final usage in the terminal snapshot so cancellation/full progress queues cannot drop it. The TUI applies accounting after join while suppressing late model text and retaining a cancelled outcome. Full race validation passed 562 tests/subtests; two regressions passed 20 repetitions (40 passes), and vet passed. See [scope](session-usage-cancel-integration.md), [aggregate patch](session-usage-cancel-integration.patch), and [hashed evidence](session-usage-cancel-integration.json). Background producer joining and full acceptance remain pending.

## Manual compaction accounting

Shared durable usage observation now covers Controller.Compact. Worker results refresh reported totals and unknown attempts, including after cancellation. A successful compaction/reopen test and cancelled-summariser test passed 20 repetitions (40 passes); the full race suite passed 564 tests/subtests without failures/skips, and vet passed. See [scope](session-manual-usage-integration.md), [aggregate patch](session-manual-usage-integration.patch), and [hashed evidence](session-manual-usage-integration.json). Background/shutdown lifecycle and full acceptance remain pending.

## Manual compaction shutdown join

Registered manual workers now retain deferred command dispatch while supporting cancellation/join during terminal shutdown. Tests cover undispatched commands and an active provider held after cancellation. Full race validation passed 566 tests/subtests; shutdown/manual checks passed 20 repetitions (160 events), and vet passed. See [scope](session-compaction-shutdown-integration.md), [aggregate patch](session-compaction-shutdown-integration.patch), and [hashed evidence](session-compaction-shutdown-integration.json). Background compaction and full acceptance remain pending.

## Background compaction completion fence

Local Harness 0581441 joins run-owned background compaction before EventDone and final stream closure, retaining cancellation and usage observation. Held-provider completion/cancellation tests passed 20 race-enabled repetitions; full Harness/Hand suites passed 881/566 tests and subtests with no failures/skips, and vet passed. See [scope and remaining error-path review](harness-background-join.md), [patch](harness-background-join.patch), and [evidence](harness-background-join.json). Full acceptance and released-dependency integration remain pending.

## Completed background compaction result retention

Local Harness 3b7fe5e retains context-owned results until joining, preventing early-finished errors from becoming successful EventDone. Two regressions failed before the fix. Final targeted tests passed 20 repetitions (100 events), and full Harness/Hand race suites passed 883/566 tests/subtests; vet passed. See [scope](harness-compaction-result.md), [patch](harness-compaction-result.patch), and [evidence](harness-compaction-result.json). This resolves the fast-completion error-path gap identified in the previous slice; full acceptance remains pending.

## Durable usage origins and legacy uncertainty

Staged Hand now persists the start of usage tracking, retains uncertainty for older history after new attempts/restart, and carries that state through resume, compaction and cancelled terminal snapshots. Fork accounting starts at zero before inherited history is copied. Full race/coverage tests passed 572 tests/subtests; targeted checks passed 20 repetitions (240 events), and vet passed. See [scope](session-usage-origin-integration.md), [aggregate patch](session-usage-origin-integration.patch), and [evidence](session-usage-origin-integration.json). Full milestone and released-dependency acceptance remain pending.

## Durable run outcomes and model history

Staged Hand records durable goal starts/final outcomes with requested model metadata, preserving unfinished starts after restart. Metadata-write failures prevent successful completion; schema/transition validation prevents appending to an invalid journal. Full race/coverage validation passed 584 tests/subtests; journal and transition checks passed 20 repetitions (240 events total), and vet passed. See [scope](session-run-journal-integration.md), [aggregate patch](session-run-journal-integration.patch), and [evidence](session-run-journal-integration.json). Full acceptance remains pending.

## Attachment storage foundation

Added an unpublished Harness content-addressed attachment store with bounded blobs/quota, durable exclusive publication and verified reads. Tests passed 20 race-enabled repetitions; full Harness validation passed 888 tests/subtests, vet passed, and four target builds compiled. See [scope and remaining integration](harness-attachment-store.md), [patch](harness-attachment-store.patch), and [evidence](harness-attachment-store.json). Session references/replay/fork/export remain pending; this does not close attachment workflow acceptance.

## Session attachments integrated in development

Persistent image records now use verified blob references, with runtime hydration and explicit missing-image failures. Staged forks copy required blobs; portable exports include sibling .attachments content and enforce the aggregate export limit. Large-image restart/fork/export tests passed 20 repetitions; final Harness/Hand full race suites passed 894/586 tests/subtests and both vet checks passed. See [scope, portability and remaining migration work](session-attachments-integration.md), [Hand patch](session-attachments-integration.patch), and [evidence](session-attachments-integration.json). Full acceptance remains pending.

## Bounded oversized legacy-image import

Harness now normalizes oversized inline image records under migration-only limits, retains the exact original backup/digest, and validates small reference-backed output with the normal parser. Staged Hand manager import/resume restores image bytes and unknown legacy usage. Regressions fixed omitted legacy payload compatibility and the exact 32 MiB image boundary. Final Harness/Hand suites passed 900/587 tests/subtests; both vet checks passed. See [scope and remaining combined-tail recovery](session-image-migration-integration.md), [patch](session-image-migration-integration.patch), and [durable evidence](session-image-migration-integration.json). Full acceptance remains pending.

## Combined migration and tail recovery

One leased operation now validates legacy clones/images and repairs an unfinished tail while backing up the complete original. Staged Hand uses that operation and preserves backup diagnostics. Harness/Hand full suites passed 908/588 tests/subtests; both vet checks passed. Twenty repeated recovery checks included 40 killed child processes. The previous Hand recovery path fails the new regression. See [scope](session-combined-recovery-integration.md), [aggregate patch](session-combined-recovery-integration.patch), and [durable evidence](session-combined-recovery-integration.json). Full acceptance remains pending.

## Complete session listing metadata

Replaced silent partial scan results with explicit per-file metadata errors while preserving discovery for recovery. Large legacy records now yield complete counts/timestamps. Harness/Hand race suites passed 910/588 tests/subtests, metadata checks passed 20 repetitions, and both vet checks passed. See [scope](harness-list-metadata.md), [patch](harness-list-metadata.patch), and [evidence](harness-list-metadata.json). Full acceptance remains pending.

## Usage journal writes preserve readable accounting

Conflicting/repeated IDs, malformed existing records and aggregate overflow are handled before mutation, with concurrent provider callbacks serialised. Prior-source regressions fail; final race/coverage suite passed 594 tests/subtests, repeated checks passed and vet passed. See [scope](session-usage-write-integration.md), [patch](session-usage-write-integration.patch), and [evidence](session-usage-write-integration.json). Full acceptance remains pending.

## Application input queue foundation

Added bounded editable/cancellable queues and atomic follow-up admission after active work settles. Twenty repeated checks passed; full race suite passed 597 tests/subtests and vet passed. See [scope](input-queue-integration.md), [patch](input-queue-integration.patch), and [evidence](input-queue-integration.json). TUI dispatch, tool-boundary steering and full M3.2 remain pending.

## Terminal follow-up controls

Added queue commands, automatic dispatch after successful settled goals, and explicit resume after cancellation. Slash queue input no longer answers tool approvals accidentally. Twenty repeated TUI checks passed; full race suite passed 600 tests/subtests and vet passed. See [scope](input-queue-tui-integration.md), [patch](input-queue-tui-integration.patch), and [evidence](input-queue-tui-integration.json). Steering/attachments/durable reconciliation and full acceptance remain pending.

## Harness safe steering boundary

Added optional serial tool-boundary steering with explicit skipped call/result pairs, durable correction IDs and acknowledgement after flush. Twenty repeated checks passed; full Harness/Hand race suites passed 913/600 tests/subtests and both vet checks passed. See [scope](harness-steering-boundary.md), [patch](harness-steering-boundary.patch), and [evidence](harness-steering-boundary.json). Hand steering integration and full acceptance remain pending.

## Hand steering delivery

Connected queued corrections to Harness through immutable claims and idempotent acknowledgement; added terminal /steer and claimed-state display. Integrated provider/tool and terminal tests passed 20 repetitions; full Hand race suite passed 603 tests/subtests and vet passed. See [scope](steering-integration.md), [patch](steering-integration.patch), and [evidence](steering-integration.json). Failure reconciliation, restart/attachments and full acceptance remain pending.

## Steering claim reconciliation after join

Confirmed writes remove claims; confirmed absence restores editability. Failed persistence, identity mismatch or conflicting payloads leave the queue unchanged. Twenty repeated checks passed (140 events); full Hand race suite passed 610 tests/subtests and vet passed. See [scope](steering-reconciliation-integration.md), [patch](steering-reconciliation-integration.patch), and [evidence](steering-reconciliation-integration.json). Process-kill/queue restart, attachments and full acceptance remain pending.

## External-editor input workflow

Ctrl+G edits current input through VISUAL/EDITOR with private temporary storage and explicit validation. Failures and textarea truncation preserve the original draft. Final repeated checks passed; full Hand race suite passed 619 tests/subtests and vet passed. See [scope](external-editor-integration.md), [patch](external-editor-integration.patch), and [evidence](external-editor-integration.json). Native interactive qualification, process-kill cleanup and full acceptance remain pending.

## Strict image input and visible attachment failures

Normal terminal prompts now support quoted/escaped image paths and atomic attachment validation. Failed attachments retain the draft and do not start a goal. Twenty repeated checks passed; full race suite passed 623 tests/subtests and vet passed. See [scope](image-input-integration.md), [patch](image-input-integration.patch), and [evidence](image-input-integration.json). External policy, queued/CLI images, file references/completion and full acceptance remain pending.

## One-shot image-input parity

One-shot execution now uses strict image parsing and image-capability admission, with explicit failures before provider calls/journal writes. Twenty repeated integration checks passed; full Hand race suite passed 627 tests/subtests and vet passed. See [scope](cli-image-integration.md), [patch](cli-image-integration.patch), and [evidence](cli-image-integration.json). Standalone binary image qualification, external policy, queued images and full acceptance remain pending.

## Explicit workspace file references

Shared prompt parsing now includes bounded UTF-8 @file snapshots with atomic errors and stable source text. Twenty repeated checks passed; full Hand race suite passed 631 tests/subtests and vet passed. See [scope](file-reference-integration.md), [patch](file-reference-integration.patch), and [evidence](file-reference-integration.json). External policy, queue integration, path completion and full acceptance remain pending.

## Asynchronous workspace file completion

Tab completion now handles quoted paths, cursor-local replacement and stale-result rejection with bounded rooted directory scans. Final repeated checks passed; full Hand race suite passed 634 tests/subtests and vet passed. See [scope](file-completion-integration.md), [patch](file-completion-integration.patch), and [evidence](file-completion-integration.json). External policy, queued attachment resolution and native/final qualification remain pending.

## Queued follow-up attachment admission

Follow-ups now resolve files/images outside the owner lock and revalidate their queue entry before admission. Failed reads/profile rejection or concurrent edits preserve pending input. Twenty repeated checks passed; full Hand race suite passed 637 tests/subtests and vet passed. See [scope](queued-attachment-integration.md), [patch](queued-attachment-integration.patch), and [evidence](queued-attachment-integration.json). Steering attachments, external policy and full qualification remain pending.

## Image-capable Harness steering

Added image persistence and semantic retry comparison to the steering boundary. Twenty repeated checks passed; full Harness/Hand race suites passed 915/637 tests/subtests and both vet checks passed. See [scope](harness-steering-images.md), [patch](harness-steering-images.patch), and [evidence](harness-steering-images.json). Hand snapshot integration, adversarial hydration review and full acceptance remain pending.

## Steering attachment retry snapshots

Implemented immutable steering attachment snapshots and bounded Harness replay identity checks. Hand full race suite: 639 passing test/subtest events; Harness: 921. Focused suites: 220 events each over 20 repetitions; vet passed. See `steering-snapshot-integration.json` for preserved raw evidence and hashes. Integration remains staged; M3.2 remains in progress.

## Exact external attachment grants

Added invocation-local `--allow-attachment PATH` snapshots for explicit @ references in normal input, steering and follow-ups. Grants do not change tool access. Regressions cover immutable snapshots, exact-path and explicit-reference restrictions, FIFO/directory rejection, cancellation, text limits and one-shot provider admission. Full race suite and vet passed; the subsequent one-shot regression ran separately. Raw evidence and aggregate sources are recorded in `external-attachment-integration.json`. M3.2 remains in progress.

## M3.3 full captured output viewer

Tool results now retain typed raw content independently of previews. `/output [number]` opens a bounded scrolling view with explicit Home/End and resize support. Live and replay use the same sanitised preview and retained content, including failed-tool partial output. The TUI race suite and vet passed after fixing missing End navigation; the initial failing evidence is retained. See `output-viewer-integration.json`. Search/copy, other block types, file capture expansion, dedicated diffs and performance qualification remain pending.

## Output search and copy

Added bounded literal search with next/previous matching-line navigation and wrapped-row targeting. The viewer sends explicit OSC52 clipboard requests and reports write failures without claiming clipboard acceptance. TUI race tests and vet passed; native terminal clipboard acceptance remains unqualified. Evidence: `output-search-copy-integration.json`.

## Approval preview navigation

Approval panels are bounded to eight rows at 80x24. The v key opens the full preview in a scrollable, searchable viewer without answering approval; Escape returns to the decision. Paths are visible and distant edits produce separate unified hunks. Preserved decision labels after an initial regression, and bounded long tool names so decision controls remain visible. Agentio/TUI race suites and vet passed. Evidence: `approval-viewer-integration.json`. Large omitted diffs and native qualification remain pending.

## Incremental transcript layout

Replaced full-history wrapping and viewport scans with cached per-block rows and a cumulative row index. Only changed blocks rewrap; View renders the visible interval. Mutation, resize, Unicode, replacement and scrolling regressions pass. Added a reproducible 10,000-block benchmark: 30 runs and 1,020 events, measured through View construction. Raw metrics and limitations are in `transcript-cache-integration.json` and `transcript-cache.md`. No acceptance group is closed.

## Assistant source retention and Markdown reflow

Added typed assistant source blocks for live flush, history load and session switching. Rendered Markdown is cached by width/style and rebuilt from source on resize. Source identity survives banner insertion and is cleared on screen/session replacement. TUI race regressions and vet passed. Other transcript kinds and asynchronous resize layout remain pending. Evidence: `markdown-source-integration.json`.

## Typed events and application output-viewer correction

Migrated user and tool-call/result rendering plus replayed compaction/note records to typed source blocks. Fixed the production application event path, which previously failed to populate the output viewer despite the legacy runtime path doing so. Truncated application details now remain visibly labelled in previews and the viewer. Application/replay parity and source retention regressions pass, alongside TUI race tests and vet. Evidence: `typed-events-integration.json`. Approval/status migration and complete artifact retrieval remain pending.

## Full persisted result retrieval

Added session/tool-ID checked controller retrieval and automatic viewer loading for truncated application details. Missing and duplicate identities fail explicitly; stale load results cannot replace another viewer. Tests cover a 260KB persisted result and final-line navigation. Application tests passed in the combined run; TUI passed after fixing test controller setup to match production. Initial sandbox HTTP failure and all test failures are retained. Evidence: `full-result-integration.json`. External artifacts and output-load lifecycle qualification remain pending.

## Output-load ownership

Output loads now start under model ownership, cancel on viewer close/replacement, and are joined on shutdown even when their Tea result is never consumed. At most four unfinished loads may exist. Result handling joins before publishing and removes ownership; stale results remain rejected. Controller lock acquisition is cancellation-aware. TUI regressions, focused lifecycle tests and a controlled lock-wait test passed; vet passed. Evidence: `output-lifecycle-integration.json`. Native qualification and external artifacts remain pending.

## Verified captured-output reader

Harness candidate 1b79b1d12a8e08b06ee49251b2b5c9a1293a2e54 adds capture SHA-256 finalisation, file sync and a store-scoped verified reader. Reads reject invalid records, symlinks, nonregular/changed files, active capture locks and mismatched content. Process/bash race suites and vet passed. Raw evidence is in `harness-artifact-reader.json`. Persisting references and wiring the Hand viewer remain pending; no full group is closed.

## Persisted output artifact references

Harness candidate 698a0c24d4fdf1fd8a17813b3224d252281470a6 persists typed stdout/stderr capture references in tool-result records for serial and streaming dispatch paths. A real command regression writes 70,000 bytes, reopens the session and verifies every captured byte via the stored digest. Arbitrary JSON/prose paths are not promoted to references. Targeted runtime/session race tests and vet passed. Hand viewer wiring and export/fork artifact handling remain pending. Evidence: `harness-artifact-persistence.json`.

## Verified artifact viewer integration

Added explicit stdout/stderr selection to the output viewer. The controller retrieves only persisted references through its configured capture store; replay and session switching preserve lookup identities. Expired/missing files, unavailable streams and binary data fail visibly. Capture-limit truncation remains labelled. A reopened-session regression verifies complete display and retention failure. Full Hand race suite and app/TUI vet passed against the new Harness candidate. Evidence: `artifact-viewer-integration.json`. Native and release qualification remain pending.

## Typed operational states

Approval request/decision records now retain source fields, IDs and preview data. Live/manual compaction retains state, summaries and token counts. Runtime errors and terminal outcomes use typed sanitised rendering. Source/state resize and unsafe-error regressions pass with the TUI race suite; vet passed. Evidence: `typed-state-integration.json`. Administrative string paths and final typed-source consolidation remain pending.

## Authoritative typed transcript store

Consolidated every production transcript write behind typed source APIs. Administrative messages, banners, clear and session replacement now preserve source ownership; stale rendered cache content cannot override it. An AST regression prevents production writes outside the store. TUI race suite and vet passed, and the 10,000-block benchmark was rerun with typed notice blocks for 30 runs and 1,020 events. Evidence: `transcript-store-integration.json`. Markdown-heavy resize and native qualification remain pending.

## Owned asynchronous Markdown resize

Histories with at least eight stale assistant blocks or 64KiB of stale Markdown now reflow in one owned worker. Obsolete width/style work is cancelled; completion joins before applying matching source/epoch results. Clear and shutdown prevent stale restoration and join outstanding work. TUI race tests for rapid resize, editable input, clear and shutdown passed; vet passed. Evidence: `markdown-layout-integration.json`. Initial rendering and single-block renderer cancellation remain synchronous limitations.

## Bounded presentation batching

The application stream now groups immediately available text events into one UI refresh, with at most 32 events and bounded text collection. Collection stops at the first non-text event and never waits to fill a batch. Exact text, approval boundaries, event/byte bounds and immediate delivery regressions passed with the TUI race suite; vet passed. Evidence: `presentation-batch-integration.json`. Legacy forwarding and native qualification remain pending.

## M3.4 owned process handles

Harness candidate 540dd08a6975ac0076f697668ca861468715cf44 adds asynchronous process-group handles with snapshots, bounded stdin, cancellation and joined completion. Uses existing bounded prefix/disk capture. Tests cover input exchange, 70KB capture, owner cancellation, input limits and a blocked input deadline that cancels/joins the process. Process suite and vet passed; initial handle tests passed 20 repetitions before the additional blocked-input test. Hand registry/UI and MCP startup remain pending. Evidence: `harness-process-handles.json`.

## Background process registry

Added bounded application process ownership with read, send, cancel, wait, forget and joined shutdown. Three lifecycle tests passed twenty repetitions under the race detector. Initial fixture permission failures are retained in evidence. Production controls and policy integration remain pending. Evidence: `process-registry-integration.json`.

## Interactive background process controls

Interactive startup now creates the registry and shutdown joins its processes and pending control operation. /process supports explicit user start/list/read/send/cancel/wait/forget, including during foreground work. New permanent TUI tests exercise interactive I/O, cancellation and late completion after shutdown. Affected app, TUI and CLI race suites and vet passed. Model tools, MCP startup and final M3.4 qualification remain pending. Evidence: `process-controls-integration.json`.

## Model-facing process tool

Registered process controls in interactive and one-shot startup, backed by the same invocation-owned registry. Mutating actions require existing runtime approval; read/list/wait are read operations. Wait is bounded to 30 seconds and exposes current state on timeout. Permanent tests cover permission classification, malformed input rejection and real shell I/O. Evidence: `process-tool-integration.json`. MCP startup and native qualification remain outstanding.

## Joined MCP construction timeout

Replaced the abandoned construction goroutine with Harness context-aware construction. Timeout now returns after partial connections are closed and joined. Actual fixture tools verify successful catalogue publication and no partial publication on failure. Agentio race suite and vet passed. Optional servers and UI-first startup remain pending. Evidence: `mcp-joined-integration.json`.

## Optional MCP degradation

Harness now supports bounded optional connections and immutable server status; required failures and owner cancellation remain fatal. Hand maps optional configuration and shows status in interactive and one-shot modes. Healthy-only catalogue regression passed. Evidence: `harness-mcp-optional.json` and `mcp-optional-integration.json`. UI-first construction remains pending.

## Cancellable interactive startup screen

Interactive MCP construction now runs behind a visible terminal startup state accepting cancellation. Ownership joins construction on cancellation, disconnect and failure; success transfers the Runtime to the main UI. CLI race suite and vet passed. This is intermediate: the editor remains unavailable until construction, and independent connection/retry controls remain pending. Evidence: `startup-screen-integration.json`.

## Independent bounded MCP connections

Harness now connects up to four servers concurrently, joins every worker, cancels siblings on required failure and publishes successful tools in configuration order. Targeted concurrency tests and full Harness race suite passed after correcting diagnostic casing exposed by the first full run. Both raw runs are retained. Affected Hand startup suites passed before that diagnostic-only correction. Evidence: `harness-mcp-concurrent.json` and `mcp-concurrent-integration.json`. Editor availability and retry controls remain pending.

## Startup draft and required-failure retry

Startup accepts an editable draft and hands it to the main editor without submission. Required failure remains visible; Ctrl+R retries joined construction, and stale completion messages cannot settle a newer attempt. Draft is preserved across retry and transfer. CLI/TUI race evidence and focused long-input regression are in `startup-draft-integration.json`; initial build failure is retained. Per-server retry, local work during optional connections and native qualification remain pending.

## Idle MCP attachment API

Harness can transfer a discovered MCP client into an idle runtime, atomically register its tools, refresh static prompt hints and retain it for joined shutdown. Busy and failure paths retain caller ownership. Registry batch collisions publish nothing. Targeted race tests and vet passed; initial test-oracle failure retained. Hand background discovery wiring remains pending. Evidence: `harness-mcp-attach.json`.

## Background optional MCP discovery

Interactive startup builds required servers, then opens the main editor while optional discovery runs independently. The manager bounds connection concurrency, retains ready clients until an idle application boundary, transfers them through Harness attachment and joins untransferred work on exit. /mcp shows status and /mcp retry <server> retries failures. Real stdio tests verify deferred catalogue publication and transferred shutdown ownership. Affected race suites and vet passed; full/native acceptance remains pending. Evidence: `mcp-background-integration.json`.

## MCP ownership audit and full integration checks

RunTurn now shares the runtime exclusion lock used by Run and MCP attachment. Optional retries remain unavailable until untransferred client cleanup joins. A new real stdio regression proves ready-client shutdown before tool publication. Full Harness and Hand race suites passed; initial Hand fixture deadline failures are retained. Healthy-child startup tests now use the configured deadline while the short hung-server bound remains unchanged. Evidence: `harness-mcp-audit.json` and `mcp-audit-integration.json`. Native/released acceptance remains pending.

## M4 protocol framing foundation

Added public versioned wire types and bounded UTF-8 JSONL framing with strict envelope validation, duplicate-field rejection and poisoned partial-write handling. Permanent tests cover frame boundaries and concurrent writes. Parser fuzz output and vet evidence are retained in `protocol-framing-integration.json`. Contract: `docs/protocol/v1.md`. RPC dispatch and CLI event publication remain unfinished.

## One-shot JSONL application events

--jsonl -p emits versioned application progress and one joined terminal event, preserving sequence and session/run identities with invocation-unique event IDs. Output failure cancels and joins the run. Untrusted workspace grants never read hidden stdin trust input in JSONL mode. Permanent tests cover ordering, terminal uniqueness, output failure and permission loading. Evidence: `jsonl-events-integration.json`. RPC/backpressure/replay work remains pending.

## Durable RPC request ledger

Added exclusively locked, bounded, fsynced request intent/acceptance/completion records. Repeated IDs return existing records and incomplete requests reopen as uncertain without an execution grant. Concurrent duplicates receive one grant. Tests cover copied completed results, conflicting IDs, writer exclusion and truncated tails. Initial nil-parameter failures were fixed by normalising omitted parameters before validation. Evidence: `rpc-ledger-integration.json`. Dispatcher wiring, kill/recovery qualification and reconciliation remain pending.

## RPC ledger process-kill recovery

Permanent subprocess tests now terminate the writer after synced pending intent, accepted run binding and completed result. Reopening reacquires the abandoned lock and never grants duplicate execution; incomplete states become uncertain and completed results survive. The ledger race suite and vet passed. Evidence: `rpc-crash-integration.json`. Power-loss durability and dispatcher integration remain pending.

## RPC application dispatcher

Added capability negotiation, state, prompt, cancel, request lookup and bounded event polling against the application service. Prompt admission uses the durable ledger and duplicate IDs return existing runs. Progress retention is bounded with explicit cursor gaps; terminal events persist before completion retrieval. Dispatcher close cancels and joins active work. Permanent tests cover duplicate execution, retention and disconnect. Evidence: `rpc-dispatch-integration.json`. Transport and remaining controls are pending.

## Owned stdio RPC transport

--rpc now serves the initial dispatcher over stdin/stdout without TUI or hidden trust input. The transport bounds queued requests, interrupts blocked I/O on cancellation, applies response write deadlines and joins reader/application work. Real duplex-pipe tests cover disconnect and slow consumers. A built CLI hello/EOF smoke returned one valid JSON response and exited successfully without model calls. Evidence: `rpc-transport-integration.json`. Remaining controls and complete journeys are pending.

## RPC approval capabilities

Added approval.pending and approval.respond with explicit decision validation and matching opaque approval/run identities. Pending approvals are retained separately from the evictable progress ring; terminal cleanup removes stale capabilities even if persistence fails. Tests exercise real application approval hooks with once/deny/cancel and reject mismatched run IDs. RPC race suite and vet passed. Evidence: `rpc-approval-integration.json`. Full client journey and remaining controls are pending.

## RPC session and profile controls

The production controller dispatcher now advertises session.list/new/select/name and profile.list/select. Mutations reuse idle reservations and reject active RPC runs. Session listing is paginated and byte bounded. Permanent tests verify new/resume history preservation, profile application and active-run rejection. Affected suites, strengthened active-run test and vet passed. Evidence: `rpc-controls-integration.json`. Steering and complete client journeys remain pending.

## RPC steering and follow-up queue controls

Added steer, followup.enqueue/start and queue.list/edit/remove through existing application queues. Explicit follow-up start uses the durable request ledger, preserving single execution and queue order on retry. Listing is bounded. Transport cancellation and input EOF cancel the dispatcher before waiting for dispatch locks, unblocking attachment resolution. Queue and disconnect regression tests passed under race detection; vet passed. Evidence: `rpc-queue-integration.json`. Automatic continuation and full provider journeys remain pending.

## RPC initial input resolution

RPC prompts now use Service.StartInput and the configured attachment resolver, matching follow-up resolution. Resolution occurs outside the owner lock; selected session and current input capabilities are checked before admission. Tests verify real workspace @file inclusion, session-change rejection and image-capability rejection. RPC/application race suites and vet passed after enabling existing local HTTP fixtures; the sandbox failure is retained. Evidence: `rpc-input-integration.json`. Full image journeys remain pending.

## RPC automatic follow-ups

Opt-in automatic follow-ups execute FIFO after durable successful terminal persistence. Cancellation and failures pause automatic execution until explicit resume. Regression tests cover predecessor persistence, order, cancellation, enqueue while paused, and completed request replay with an automatic queue head. Development evidence: `rpc-auto-integration.json`. Queue intent remains connection-local and enqueue replay deduplication remains pending. No acceptance group is closed.

## Public RPC Go client

Added sdk.Client with serialized concurrent calls, context cancellation that closes uncertain transport, response identity validation and explicit connection ownership. Tests use the real RPC dispatcher for prompt/cancel/terminal retrieval and net.Pipe for blocked response cancellation and concurrent callers. An external module builds with a local Hand replacement. A subprocess negotiation example is included. Evidence: `sdk-rpc-integration.json`. Embedded SDK and released-package acceptance remain pending; no requirements are closed.

## SDK subprocess ownership

Added StartProcess with explicit arguments/environment/directory, lifetime context, graceful pipe closure, bounded fallback termination and process reaping. Updated the RPC example to use it. Tests verify a child cleanup marker before normal exit and forced shutdown of an uncooperative child. SDK race suite and vet passed. An external module negotiated with an actual freshly built Hand process and closed cleanly, without model calls. Evidence: `sdk-process-integration.json`. Released-package and embedded API gates remain pending.

## Embedded SDK runtime

Added sdk.Open with explicit workspace/store/model, shared Hand runtime and registry, session catalogue, approval broker, background processes and RPC dispatcher. Provider construction is now shared by CLI and SDK. Close joins dispatcher work and releases runtime/session resources. Tests cover real-runtime prompt execution through a local HTTP provider fixture, session switching and reopening, and pre-cancelled construction. SDK/CLI race suites and vet passed after granting local fixture listener access; the sandbox failure is retained. Evidence: `sdk-embedded-integration.json`. Custom MCP/hooks/profile configuration, released-package qualification and full interface equivalence remain outstanding.

## SDK request recovery across sessions

Unified CLI RPC and embedded SDK on OpenWorkspaceLedger. Embedded construction acquires the ledger before creating sessions. Real-runtime SDK regression switches session after a completed provider-fixture prompt, closes/reopens, retrieves the original terminal and rejects same-ID replay in a different session without a second provider call. Workspace tests verify exclusive ownership, uncertain-intent reopening and isolation. SDK/RPC/CLI race suites and vet passed. Evidence: `sdk-recovery-integration.json`. No acceptance group is closed.

## Typed SDK snapshots

Added immutable PromptRequest, Execution, Event and EventPage snapshots with typed Submit, Lookup and PollEvents methods. Mutable payloads and page slices are copied by accessors. Terminal identity, page cursor order and request limits are checked; progress loss remains explicit through Gap. Targeted SDK race tests and vet passed, including a real-dispatcher typed cancellation journey. Evidence: `sdk-snapshots-integration.json`. Final compatibility acceptance remains pending.

## External SDK compatibility runner

Added scripts/check_sdk_compatibility.py and an external-consumer fixture comparing CLI JSONL, subprocess SDK and embedded SDK against a local provider. Development execution passed with identical text and exactly one completed terminal per interface, and exactly three provider calls. Evidence includes binary/source hashes, resolved Hand/Harness modules, commands and raw outputs. The initial offline module-inventory failure is retained. Released mode refuses local replacements and mismatched binary/Harness versions but remains unexecuted. Evidence: `sdk-compat-integration.json`. Full interface equivalence and release acceptance remain pending.

## Cross-interface cancellation journey

Extended the external compatibility fixture with active provider cancellation through CLI SIGINT, subprocess SDK cancel and embedded SDK cancel. All six completion/cancellation observations passed, with one terminal per run, exactly six provider calls and three cancelled provider request contexts. CLI interrupt exit 130 is asserted. Evidence: `sdk-cancel-integration.json`. Released packages and full approval/lifecycle equivalence remain pending.

## Python RPC lifecycle journey

A non-Go client exercised real Hand approval-before-write, steering delivery, manual follow-up queueing, duplicate execution replay, protocol version errors, cancellation, disconnect cleanup and durable terminal recovery. The corrected fixture passed with five provider calls (including steering continuation) and two cancelled provider requests. Initial call-count assertion failure retained. Evidence: `python-rpc.json`. This remains development evidence, not full M4 closure.

## Durable RPC control operations

Extended the ledger with control-operation records and persisted responses for session/profile mutations, queue controls, approval responses, cancellation and follow-up resume. Duplicate controls replay the original response across session changes and restart; changed payloads conflict, and uncertain intent never reexecutes. Tests cover queue replay/reopen, uncertain controls, large bounded responses and SDK session.new replay after reconnect. Initial approval tests reused IDs for different payloads; they now use distinct request IDs. RPC and SDK/CLI race suites and vet passed. Evidence: `rpc-control-ledger-integration.json`. Full crash/release qualification remains pending.

## Control-record killed-writer recovery

Actual child processes are killed with pending, accepted and completed control records; reopening never grants reexecution and preserves completed responses. Control response identity/result shape is validated on append/reopen. Fixed persistence-error response construction to prevent both result and error fields. The final RPC race suite passed 34 tests/subtests; vet and build passed. Python approval/steering/cancel/disconnect/reconnect journey passed against the rebuilt binary. Evidence: `rpc-control-crash-integration.json`. Full native/power-loss/release qualification remains pending.

## Scoped permission model

Started M5.1 with explicit operation/resource/workspace/configuration/lifetime/provenance grants, immutable inspection and immediate revocation. Filesystem scopes resolve symlinks and reject unresolved links; command/shell/network/MCP resources use exact identities rather than prefix matching. Tests cover configuration/lifetime changes, revocation, symlink and path-prefix escapes, argument and server changes, and rejection of project-policy provenance. Permission race suite and vet passed. Evidence: `scoped-policy-integration.json`. Approval integration, external authority storage, migration and stale-preview enforcement remain pending.

## External persistent approval authority

Added an exclusively locked, bounded private grant/revocation journal outside the workspace. Grant/revoke state changes only after file sync; failures fail closed. Reopen rejects truncated data, invalid provenance/context and saved resources whose symlink resolution changed. Tests cover persistence/revocation, exclusive ownership, project-local refusal, truncation, symlink changes and failed writes. Permission race suite and vet passed. Evidence: `authority-store-integration.json`. Approval integration, migration and isolation remain pending.

Authority-store fixture correction: the initial run used insufficiently private temporary directories and was rejected. Tests now create explicit 0700 authority directories; the final permission race suite passed 21 tests with no failures/skips. Initial raw failures are retained.

## Authority revocation replay

Fixed reopening when an already revoked resource later becomes a symlink. Journal replay validates historical structure and applies revocations before resolving active resources, so revoked paths cannot block unrelated valid grants or regain authority. Active changed-resource grants still require reapproval. Permission race suite and vet passed. Evidence: `authority-replay-integration.json`. Production approval integration remains pending.

## Explicit legacy grant migration

Added immutable migration proposals fingerprinted over canonical workspace, configuration and deduplicated legacy tool names. Import requires the reviewed fingerprint and persists explicitly labelled broad legacy-tool grants; project settings/proposals alone grant nothing. Identical acknowledgement can resume partial imports without duplicates; changed scope needs a new acknowledgement. Tests verify no pre-ack authority, fingerprint binding, broad semantics, tool separation and copied inspection. Permission race suite and vet passed. Evidence: `legacy-migration-integration.json`. UI and approval-path integration remain pending.

## Scoped approval hook integration

Added resource derivation from actual tool arguments, scoped approval previews, post-decision argument/resolved-path revalidation and durable exact-resource grants. Workspace reads remain routine; external reads and unknown tools need authority. Connected the hook to the shared application approval broker and exposed opt-in SDK AuthorityDirectory with a configuration fingerprint. Targeted hook race tests, scoped SDK session reopen test and vet passed. Evidence: `scoped-hook-integration.json`. CLI migration/default cutover, stale-content checks and backend isolation remain pending.

## Stale file-approval snapshots

Scoped write approvals now include a bounded before-image fingerprint (existence, mode, size and SHA-256) and recheck it after the decision. Changes to content/mode or creation of a missing target invalidate approval. Snapshot reads reject nonregular/oversized files, use no-follow nonblocking opens and check cancellation while hashing. External write previews require read authority. Targeted race tests and vet passed. Evidence: `approval-snapshot-integration.json`. Execution-boundary race closure and full acceptance remain pending.

## Scoped permission inspection and revocation over RPC

Added permission.list with bounded pagination and permission.revoke with durable replay. Configured SDK authority is attached to the application controller and advertised during negotiation. Tests verify scope/provenance inspection, immediate denial after revocation, duplicate-response replay and scoped SDK access. RPC race suite, scoped SDK lifecycle test and vet passed. Evidence: `permission-rpc-integration.json`. CLI migration/UI and complete acceptance remain pending.

## Scoped SDK approval/revocation journey and full regression

Added a real-runtime SDK journey: persistent approval writes one file, a later session reuses that exact scope without another prompt, RPC inspection/revocation removes it, and denying the next approval preserves file contents. The fixture asserts six local provider calls. The full uncached Hand development race suite passed 771 tests/subtests with no test failures/skips; three example packages compile but have no tests. Full vet passed. Evidence: `scoped-journey-integration.json`. Final clean/released candidate, coverage and native/live acceptance remain pending.

## CLI scoped-policy cutover and explicit migration commands

CLI/TUI/RPC startup now uses external scoped authority, bound to canonical workspace and configuration. Added --permissions for credential-free inspection, --ack-legacy-permissions FINGERPRINT for explicit broad migration and --revoke-permission ID. Legacy settings are read as bounded non-authorising proposals through rooted no-follow/nonblocking access. --yes retains explicit one-shot approval without persistence. CLI race suite, vet/build, Python lifecycle and six local compatibility journeys passed. Fixtures now separate HOME from workspace so authority is outside agent-writable project storage. Evidence: `cli-scoped-integration.json`. Full M5/release acceptance remains pending.

## TUI scoped permission inspection and revocation

Added /permissions [offset] and /permissions revoke <ID>. Authority operations run off the UI thread; shutdown joins durable writes. Inspection displays 16 grants per page with bounded resource and identity labels. Regression verifies pagination during a foreground run, durable revocation through shutdown/reopen, and stale completion rejection. TUI race suite and vet passed; initial canonical-workspace fixture failure retained. See tui-permissions-integration.json. Full M5 and final acceptance remain pending.

## TUI explicit legacy permission acknowledgement

Added /permissions legacy and /permissions acknowledge FINGERPRINT. Review displays broad authority semantics, exact tools, workspace and configuration. Review alone and incorrect fingerprints grant no authority; explicit acknowledgement preserves legacy provenance and is idempotent. Startup passes the immutable proposal to the controller. TUI/CLI race tests and vet passed. Evidence: tui-legacy-integration.json. Full M5 acceptance remains pending.

## Dynamic model/profile permission binding

Approval hooks and inspection now read a consistent authority snapshot. CLI prepares a journal bound to the proposed configuration before a model/profile change commits. Failed preparation preserves the current runtime and grants. Credential references are retained for same-provider model changes; profile changes bind all effective profile fields. Original journals stay open until shutdown so inspection workers remain valid. Regressions cover model, endpoint and credential separation; switching back; failed preparation; and hooks refusing old grants. Affected race suites and vet passed after two initial implementation/fixture issues were fixed. See permission-binding-integration.json. Full M5 and released-candidate qualification remain open.

## Full development validation and profile-bound RPC journey

Full Hand race suite passed 780 tests/subtests, no failures or test skips. Fresh binary passed Python RPC lifecycle and all six CLI/subprocess/embedded completion/cancellation journeys. A new permanent profile runner verifies persistent approval under one profile, reapproval under a second, denied write preservation and original-grant reuse after switching back. Initial runner casing mismatch was fixed; raw failure retained. Evidence: bound-journeys.json. These local fixtures do not qualify the final release or live comparisons.

## M5.2 reusable execution backend and native container tests

Development Harness now provides explicit host/container backends with structured argv, bounded output, explicit environment, fixed image ID, local daemon socket, restricted workspace mount and scratch, disabled networking by default, and owned container cleanup. Three race tests passed including real cached-image Docker Desktop filesystem/network/environment/cancellation checks; no test skips. Initial interface-name assumption failed and was corrected to test routes/connectivity. Vet passed; no test containers remained. Hand routing, Linux-host and full acceptance remain pending. See harness-execution.json.

## Shell tool execution boundary and full captures

Harness BashTool accepts an explicit backend and sends exact approved shell source without host path substitutions. Policy rejection precedes backend invocation; backend failure has no host fallback. Execution results now retain bounded previews, byte/truncation metadata and complete artifact captures. Native Bash-tool test verifies a container write, denied root write, 100 KB capture and integrity-checked artifact read. Affected race suites and vet passed. See harness-execution-shell.json; Hand-wide routing and full acceptance remain open.

## Container implicit mount hardening

Container execution now inspects the pinned image before creation and rejects image-declared volumes, malformed metadata and truncated inspection. Workspace binds disable recursive submount inclusion. Native execution/shell race tests and a cached PostgreSQL-image rejection test passed; the latter inspects only and never runs PostgreSQL. Vet passed. Adversarial nested-mount testing on a native Linux host and Hand-wide routing remain pending. Evidence: harness-execution-mounts.json.

## Built-in tool worker for container execution

Added a private single-request tool worker using the same Hand built-ins, bounded input/output, strict envelope validation and an explicit image payload field. It does not load model configuration or grants. Regression tests cover real file roundtrip, rejected envelopes before side effects, cancellation, short output writes and image preservation. Race tests, vet and Linux arm64 build passed. Native container file/readonly/symlink journey evidence is recorded separately. Parent backend routing and release packaging remain pending.

## Parent-side built-in tool proxy

Added backend-only tool execution wrapper preserving original definitions/concurrency while never invoking host Execute. It bounds requests/responses, rejects failed/truncated/malformed results and restores images. Harness supports a read-only worker executable mount and up to 16 MiB bounded protocol capture. Real container tests verify writes, reads, large responses and missing-worker failure; affected race/vet checks passed. Startup selection, path mapping and full isolation routing remain pending. Evidence: tool-proxy-integration.json.

## Canonical workspace mapping and exact worker file paths

The tool proxy now maps workspace file/search paths into /workspace while preserving other argument values. Canonical outside paths, traversal and escaping symlinks are rejected before backend invocation. Worker file tools use Harness ExactPath mode to disable home/Unicode spelling recovery. Native proxy/worker race tests, file-tool race tests and affected vet passed. Startup selection and atomic post-approval mutation race closure remain pending. Evidence: mapped-worker-integration.json.

## Owned background container execution

Harness execution backends now support asynchronous process handles with explicit environments, input/output capture and cleanup joined before terminal state. Hand NewProcessesWithBackend selects this path without host fallback; ProcessTool reports the configured boundary. Native background test verifies input, captured output, root-write denial and shutdown of child work. Full app/TUI and affected Harness race suites plus vet passed. Startup selection and other boundary integration remain pending. Evidence: background-backend-integration.json.

## CLI container selection and effective boundary reporting

User execution configuration now selects host/container mode. Startup validates required immutable image and absolute paths, probes Bash/worker before runtime construction, routes built-ins through the worker and background processes through owned backend handles. MCP/hooks need explicit external trust. Startup stderr, RPC hello and /boundary report the effective scope; model instructions explain relative paths. Final built CLI passed the local Python lifecycle with container execution enabled. SDK selection, packaging/integrity pinning and full acceptance remain pending. See isolation-startup-integration.json.

## Pinned worker executable snapshots

Container configuration now requires worker_sha256. Harness opens a regular executable without following the final symlink, copies and hashes bounded bytes into private state outside the workspace, and mounts the verified copy. Original-path replacement cannot change mounted content. Mismatches/cancellation remove incomplete snapshots. Snapshot/configuration/proxy race tests and vet passed, and the built CLI passed the isolated RPC lifecycle. SDK selection, packaging and final acceptance remain open. Evidence: worker-pin-integration.json.

## Embedded SDK container selection

EmbeddedOptions.Execution now uses the same probes, pinned worker, built-in proxy and background backend as CLI startup. Container mode requires external scoped authority; execution options participate in the authority digest. RPC reports the scope and model instructions explain container-relative paths. Full SDK race suite and vet passed with native fixtures, including real approval/reuse/revocation/denial and wrong-digest rejection. Release packaging and final acceptance remain pending. Evidence: sdk-isolation-integration.json.

## Container worker archive packaging

Added four-platform archive builder bundling Linux workers for matching architectures, with WORKER.json containing SHA-256, release identity and protocol version. Builder requires a fresh output directory. Strict smoke mode checks ELF architecture, executable mode and manifest integrity. Four development archives passed structural checks; macOS arm64 CLI passed native smoke, packaged Linux arm64 worker passed Docker health, and five regression tests passed. Makefile/release strict-mode wiring is staged in worker-dist-integration.patch after the SDK aggregate. No release was published; other native runtimes and final acceptance remain open. Evidence: worker-dist-integration.json.

## Packaged CLI and worker integrated journey

Permanent archive runner checks worker ELF/hash and CLI release identity before using both extracted binaries for the local RPC lifecycle. Final run passed nine scenarios, five local provider calls and two cancelled requests. Added packaging/configuration/verification documentation. Evidence: packaged-isolation.json. This covers macOS arm64 with a Linux arm64 Docker worker; remaining native platforms and full acceptance are still open.

## M5.3 bounded workspace snapshot foundation

Added non-mutating Git-independent capture of regular-file bytes, SHA-256, size and permissions, immutable content access and before/after change comparison. Sensitive paths and symlinks are explicit omissions; file/total/entry/depth limits fail whole capture. Rooted access, regular-file identity checks and nonblocking opens reject substitution and special-file hazards. Batched directory enumeration bounds traversal allocation. Final race suite passed eight tests/subtests; vet passed. Persistence, retention, restore, verification records and runtime/UI integration remain pending. Evidence: checkpoint-snapshot-integration.json.

## Durable bounded checkpoint store

Added external private checkpoint storage, exclusive process ownership, sync-before-rename publication and directory-sync durability. Load validates record structure, content hashes, scope and snapshot digest. Storage count/byte limits preserve accepted checkpoints until explicit deletion. Recovery removes only recognised private unpublished temporary files under the process lock; suspicious pending paths fail closed. Final checkpoint race suite passed 15 tests/subtests without failures/skips; vet passed. Restore, provenance, export, UI/runtime integration and process-kill/fault qualification remain pending. Evidence: checkpoint-store-integration.json.

## Snapshot-bound verification command records

Added execution through concrete workspace-bound host/container backends, durable before/after snapshots, command/profile identity, exit/error data and bounded output/artifact references. Later file/profile changes and commands that mutate the captured scope cannot retain passed status. Failed post-capture is unverified; failed commands retain exit evidence. Final verification/checkpoint race suite passed 22 tests/subtests and vet passed. Initial test build API mismatch corrected with failure retained. Record persistence, UI/runtime integration and restore remain pending. Evidence: verification-record-integration.json.

## Selective restore conflict preview

Added explicit bounded file selection, immutable create/remove/replace previews, and expected after-image checks. Previews target the captured starting bytes/modes, preserve unrelated changes, and flag later edits, mode changes, excluded/symlink paths and unchanged selections. No filesystem mutation occurs. Final checkpoint/verification race suite passed 26 tests/subtests; vet passed. Guarded restore execution and UI remain pending. Evidence: restore-preview-integration.json.

## Real checkpoint process interruption

Added a publication seam and subprocess regression that kills a real Save after temporary-file sync and before rename. Reopen preserves the accepted snapshot, rejects the unpublished digest, removes its orphan and permits a new save without workspace mutation. Injected publication failure also preserves accepted state. Full checkpoint/verification race suite passed 28 tests/subtests; crash test passed 20 consecutive repetitions; vet passed. Other crash boundaries and final acceptance remain pending. Evidence: checkpoint-crash-integration.json.

## Checkpoint sync failures and published-process recovery

Added file-sync and directory-sync seams with explicit uncertain-durability errors and poisoned-handle behaviour. Prepublication ENOSPC preserves accepted snapshots and permits later retry; directory-sync EIO prevents continued operations until reopen. Reopen now syncs the recovered directory. A real process-kill test after rename/before directory sync recovers the published snapshot and passed 20 repetitions. Full checkpoint/verification race suite passed 31 tests/subtests; vet passed. Physical power loss/native Linux and final qualification remain open. Evidence: checkpoint-sync-integration.json.

## Durable verification record export

Added bounded versioned content-addressed evidence files in external private directories, sync-before-publication, no-overwrite hard-link publication and strict hash/schema validation on load. Binary output is base64 encoded to preserve non-UTF8 bytes. Tests cover roundtrip, failed records, tampering, cancellation, workspace rejection and symlinks. Corrected initial fixture directory modes with failures retained. Final checkpoint/verification race suite passed 34 tests/subtests; vet passed. Retention, export crash/fault tests, referenced-evidence qualification and runtime integration remain pending. Evidence: verification-export-integration.json.

## Verification reference qualification

Added evidence-backed assessment that loads referenced snapshots, compares omission scope and validates full output artifacts against byte counts and previews. Expired/deleted snapshots or output and contradictory metadata cannot remain passed. Current snapshot and profile still determine staleness. Final checkpoint/verification race suite passed 37 tests/subtests; vet passed. Runtime/UI integration and remaining M5.3 work remain pending. Evidence: verification-check-integration.json.

## Owned application checkpoint boundary

Service options now support a before/after run boundary within operation ownership. Before-capture failure prevents backend execution; after capture is joined before journal finish and terminal publication, with a bounded cleanup context. WorkspaceCheckpoints saves actual before/after snapshots and retains bounded run pair summaries. Four targeted application lifecycle/checkpoint race tests and vet passed. Startup selection, background-writer coordination, durable pair provenance and broader qualification remain pending. Evidence: app-checkpoint-integration.json.

## Checkpoint background admission guard

Workspace capture can now require all owned background handles to have joined and atomically pause new starts until capture/save finishes. Starts reject clearly during capture; the mutex is not held across disk work. Workspace identity is checked against the process owner. Tests cover blocked starts, release/idempotence, capture rejection during running background work, and explicit incomplete after-image evidence. Targeted process/checkpoint race suite and vet passed. Startup wiring and final acceptance remain pending. Evidence: checkpoint-process-integration.json.

## Durable checkpoint run associations

Added hand.checkpoint session annotations for start/finish pairs. Before-image association is persisted before boundary admission; finish association is persisted before terminal reporting. Replay retains interrupted starts and rejects duplicate/unmatched or changed-before finishes. Tests verify session close/reopen, referenced snapshot availability and admission failure on journal error. Targeted checkpoint/run-journal race suite and vet passed. Startup/UI wiring and restore remain pending. Evidence: checkpoint-journal-integration.json.

## CLI checkpoint startup selection

Added --checkpoint-dir to open a private external store and select the checkpoint boundary for one-shot, RPC and interactive shared-service runs. ConfigureCheckpoints admits only idle Harness services and binds the journal callback to the live runtime session. Store lifetime follows application shutdown. Targeted CLI/app race tests, durable binding regression and vet passed. Built-binary journeys, SDK options, changes/restore UI and full qualification remain pending. Evidence: checkpoint-startup-integration.json.

## Built CLI checkpoint RPC journey

Permanent RPC runner now accepts --checkpoints and starts both clients with the same external --checkpoint-dir. Built development CLI passed lifecycle/cancellation/reconnect using five local provider calls. Oracle verified two content-checked snapshots, the actual approved file bytes, and all three durable start/finish pairs after clients closed. Full raw fixture state is hashed in checkpoint-rpc.json. Restore/UI, SDK integration and final qualification remain pending.

## Embedded SDK checkpoint selection

EmbeddedOptions.CheckpointDirectory now configures the shared store, process guard and run/session journal, with cleanup owned by the SDK lifetime. Real runtime prompt test verifies a persisted snapshot and store reopen; workspace-directory rejection creates no checkpoint path. Three targeted SDK race tests and vet passed using local fixtures. Added checkpoint-usage.md with limits and current scope. Restore/UI and final qualification remain pending. Evidence: sdk-checkpoint-integration.json.

## Durable checkpoint change inspection

Controller CheckpointChanges reads session associations and integrity-checked snapshots under idle operation ownership, with 32-change pages and explicit errors for interrupted or missing evidence. RPC checkpoint.changes exposes this without file mutation. Application tests verify pagination, metadata, ownership and deleted snapshot rejection; RPC regression verifies response and invalid offset. Race tests and vet passed. TUI/restore and final qualification remain pending. Evidence: checkpoint-changes-integration.json.

## TUI checkpoint changes command

Added /changes [run-ID] [offset] using the existing joined session-operation worker, with active-run guarding and preserved session view. Displays recorded added/removed/changed file sizes and modes, omission counts and next-page command. Formatting/worker/lifecycle race tests and vet passed after correcting an initial test-field typo. Usage guide updated. Built PTY journey, restore and final acceptance remain pending. Evidence: tui-changes-integration.json.

## Built checkpoint changes and session reconnect

Built CLI passed checkpoint.changes inspection through RPC, including actual added-file hash, invalid offset rejection and identical response after reconnect and original-session reselection. Initial oracle omitted session reselection; failure retained and corrected. Five provider fixture calls and all three checkpoint pairs passed. Evidence: changes-rpc.json. Built PTY, restore and final qualification remain pending.

## Built terminal checkpoint inspection

Permanent PTY runner reopened the saved fixture session with --checkpoint-dir, entered /changes RUN-ID, observed the added approved.txt entry and recorded-comparison notice, then exited with /quit and status zero. Raw terminal transcript and binary/runner hashes retained in changes-terminal.json. This is a macOS 140x45 terminal inspection journey; restore and final qualification remain pending.

## Current-state selective restore review

Controller PreviewCheckpointRestore and RPC checkpoint.restore_preview now load durable before/after evidence and capture current state under application/background guards. Explicit selection is bounded to 32 files. Response includes reviewed digest, operations and conflicts without writing workspace or creating a new checkpoint. Application/RPC race tests verify removal eligibility, later-edit conflict, preserved file bytes and wire response; vet passed. Restore execution and final acceptance remain pending. Evidence: restore-review-integration.json.

## Terminal selective restore preview

Added /restore-preview RUN-ID PATH through the joined session worker, guarded during active runs. Exact paths support spaces and optional double-quote escapes without shell expansion. Proposed operation/size/mode and current-state conflicts are displayed with explicit preview-only status. Five targeted TUI race tests and vet passed. Usage guide updated; execution and final qualification remain pending. Evidence: tui-restore-review-integration.json.

## Full development regression after checkpoint integration

Full staged Hand uncached race suite passed 846 tests/subtests with no test failures/skips and container fixtures enabled. Four command/example packages have no test files and are not counted as passes. Full vet passed. Source hashes, actual temporary go.mod and raw test output are recorded in checkpoint-full.json. This is development evidence against unpublished Harness, not final release qualification. Restore and remaining M5-M8 work remain open.

## Atomic restore filesystem primitives

Added directory-descriptor-relative atomic exchange and exclusive move for macOS/Linux, with strict leaf-name validation and explicit unsupported-platform failures. Tests verify displaced bytes/modes, no-clobber move, final-symlink preservation and failed exchange safety. macOS checkpoint race suite and vet passed; four real Linux arm64 container primitive tests passed. Transaction/recovery/execution wiring remains pending. Evidence: restore-atomic-integration.json.

## Single-file restore transaction executor

Added internal create/replace/remove execution with current-file hash/mode/identity validation, bounded staging writes, synced preparation, mandatory intent callback before mutation, atomic operations and retained displaced content. Post-mutation validation reports uncertainty for reconciliation. Tests verify dirty starting bytes, executable modes, intent ordering/failure, retained recovery bytes and later-edit refusal. Checkpoint race suite passed 33 tests/subtests; vet passed. Durable transaction journal/reconciliation and user-facing execution remain pending. Evidence: restore-file-integration.json.

## Durable restore transaction journal

Checkpoint Store now supports bounded workspace-bound prepared/applied records under exclusive ownership, with strict transition replay, fsync-before-success and poisoned-handle handling of uncertain writes. Reopen validates the journal; journal bytes count against storage capacity. Tests verify an actual restore transaction across reopen, retained displaced content, duplicate rejection, malformed-tail failure and no mutation after intent-sync EIO. Checkpoint race suite passed 36 tests/subtests; vet passed. Recovery reconciliation and user-facing restore remain pending. Evidence: restore-journal-integration.json.

## Restore recovery inspection and process termination

Added read-only reconciliation of durable restore records against current target and retained recovery bytes, classifying prepared, applied-unrecorded, applied and conflicting states. Parent symlinks are rejected. Real subprocess termination tests cover create/replace/remove at three journal boundaries, verify reopened classification, file modes/content and retained displaced bytes. Full checkpoint race suite and vet passed; all nine crash cases passed 20 repetitions. This is macOS development evidence. Explicit reconciliation/retirement, concurrent external ancestor replacement, user-facing execution and final qualification remain pending. Evidence: restore-recovery-integration.json.

## Reviewed restore store execution

Added ApplyRestore for explicit conflict-free selections of up to 32 files. It holds store ownership, reloads durable before/after snapshots, recaptures the workspace and rejects stale review digests, reconstructs and validates actions, checks unresolved journal preparations and record capacity, then durably executes each file. Partial results identify attempted files and retained recovery paths on errors. Tests restore create/replace/remove selections with dirty original bytes and modes, and prove stale/tampered/cancelled plans leave workspace and journal unchanged. Checkpoint race suite and vet passed. Application ownership/UI confirmation wiring, recovery reconciliation, external-edit races and full qualification remain pending. Evidence: restore-apply-integration.json.

## Application-owned confirmed restore

Added ApplyCheckpointRestore accepting the exact explicitly confirmed preview. It reserves idle application ownership, validates current-session durable run bindings, pauses owned background admission, recaptures current workspace and compares every action before invoking durable store execution. Partial results propagate with errors. Regression verifies busy rejection, stale confirmation, altered checkpoint binding, successful selected removal, preservation of unselected user edits and rejection of old confirmation replay. Targeted checkpoint application race suite and vet passed. RPC/TUI confirmation, recovery reconciliation and final acceptance remain pending. Evidence: app-restore-integration.json.

## Durable RPC confirmed restore

Added checkpoint.restore requiring confirmed=true and the exact preview. It uses the durable control ledger before execution and persists a result with completed/files/error so partial application is visible. Tests verify missing confirmation rejection, selected-file restoration, duplicate replay, ledger reopen replay preserving a later user edit, and changed request-payload conflicts. Initial fixture lacked required session identity; corrected with failure retained. Targeted RPC/control race suite and vet passed. Built process journeys, TUI confirmation, recovery reconciliation and final qualification remain pending. Evidence: rpc-restore-integration.json.

## Built RPC restore and restart replay

Permanent local CLI runner now supports --checkpoints --restore. Three real clients verify explicit confirmation, selected removal, retained displaced content, replay before/after process restart preserving a later user edit, stale new-request rejection and exactly two restore journal records. Lifecycle fixture passed with five local provider requests and two cancellations; all three checkpoint pairs persisted. Initial oracle journal-field casing error retained and corrected. Raw fixture files, binary/runner hashes and results are archived in restore-rpc.json. TUI execution, recovery reconciliation and full qualification remain pending.

## Terminal restore confirmation

Restore previews retain the displayed exact review and offer /restore-confirm CURRENT-DIGEST or /restore-cancel. Confirmation is consumed before the joined worker calls application restore; active runs reject it, and failed/cancelled operations do not retain reusable confirmation. Attempted file and recovery-path lines are displayed even when the operation fails. Tests cover token mismatch, busy rejection, consumption/replay, discard and partial-result display. Targeted TUI race suite and vet passed. Built PTY confirmation journey, recovery reconciliation and final acceptance remain pending. Evidence: tui-restore-integration.json.

## Built terminal restore confirmation journey

Rebuilt CLI and prepared a fresh local RPC checkpoint fixture. The real 140x45 macOS PTY entered /restore-preview, observed the selected removal and full confirmation digest, verified unchanged bytes before confirmation, entered /restore-confirm, observed completion and quit with status zero. Filesystem oracle verified selected-file absence, retained displaced bytes and exactly prepared/applied journal records. Raw terminal/fixture evidence and hashes are archived in restore-terminal.json; fixture files reflect post-terminal state. Recovery reconciliation and final qualification remain pending.

## Applied restore recovery reconciliation

Added explicit ReconcileAppliedRestore under store ownership. It reloads the journal, verifies the selected intent and current target/recovery bytes, and durably appends the missing applied record without repeating filesystem mutation or deleting retained content. Repeated acknowledgement is idempotent; prepared/conflicting transactions reject. Real create/replace/remove process-kill tests now exercise acknowledgement and retry after reopen; all nine crash cases passed 20 repetitions. Full checkpoint race suite and vet passed. Prepared resolution, retirement, application/UI wiring, external-editor races and final qualification remain pending. Evidence: restore-reconcile-integration.json.

## Durable prepared restore cancellation

Added CancelPreparedRestore with strict prepared-to-cancelled journal transitions. It revalidates selected intent and unchanged target/staging bytes, records cancellation durably, and preserves all staging content. Retries are idempotent; changed/applied recoveries cannot be cancelled. Subsequent restore admission recognises resolved cancellations. Full checkpoint race suite passed; then strengthened process tests reopened resolved stores to verify durable cancelled/applied states, with all nine cases passing 20 repetitions. Vet passed. Recovery retirement, application/UI resolution wiring and final qualification remain pending. Evidence: restore-abandon-integration.json.

## Application and RPC recovery resolution

Added workspace-scoped paginated recovery inspection and explicit acknowledge/cancel resolution under idle application ownership and background admission guards. Recovery remains available across sessions. RPC checkpoint.recoveries returns up to 32 entries; checkpoint.recovery_resolve requires confirmed=true and an exact inspected record, using durable control request replay. Tests cover prepared cancellation, busy/invalid-action rejection, target preservation, RPC applied inspection/acknowledgement and resolution replay after ledger reopen. Targeted app/RPC race suite and vet passed. TUI resolution, built recovery journeys, retirement and final qualification remain pending. Evidence: app-recovery-integration.json.

## Terminal recovery inspection and resolution

Added /recoveries [offset] and /recovery-resolve acknowledge|cancel RECOVERY-ID. The terminal retains only the displayed page, offers cancellation for prepared records and acknowledgement for applied-unrecorded records, and consumes selection before the joined worker. Conflicts offer no resolution command; new session operations invalidate retained reviews. Tests cover pagination/action presentation, wrong record/action rejection, active-run rejection, consumption and retry rejection after failure. Targeted TUI race suite and vet passed. Built recovery journeys, retirement and final qualification remain pending. Evidence: tui-recovery-integration.json.

## Full development regression after restore and recovery

Full staged Hand uncached race suite passed 882 tests/subtests with zero test failures/skips; four command/example packages have no test files. Full vet passed. Cached container fixtures were enabled. Source hashes, actual temporary go.mod/go.sum and raw output are archived in restore-full.json. An unused Harness volume fixture variable was mistyped; this is not Harness volume-suite evidence. This is development evidence against unpublished Harness, not final qualification. Recovery retirement and remaining M5-M8 work remain open.

## Recovery files excluded from project checkpoints

A permanent regression reproduced retained restore bytes entering later snapshot content and ordinary change lists. Generated .hand-restore-<32 lowercase hex> names are now reserved at every directory depth, explicitly omitted from captures and denied as restore selections. Non-reserved similarly named files remain captured; recovery bytes remain unchanged. Original failure retained. Full checkpoint/verification race suites and vet passed. Recovery retirement and broader qualification remain pending. Evidence: recovery-scope-integration.json.

## Configurable checkpoint scope and limits

Added shared zero-default checkpoint options with copied exclusion slices and bounded validation. CLI exposes repeatable --checkpoint-exclude and entry/file/total/snapshot/store limits; embedded SDK exposes Checkpoints. Startup passes resolved values into capture and storage. Regression verifies exclusions, copied scope, encoded store creation, custom file-size refusal and invalid options. Targeted checkpoint tests and vet for checkpoint/CLI/SDK packages passed. Built custom-option journeys and broader qualification remain pending. Evidence: checkpoint-options-integration.json.

## Embedded runtime configured checkpoint evidence

Extended the real embedded runtime fixture with custom capture/store bounds and a private subtree exceeding the file cap. The run persisted only ordinary source bytes; excluded payload was absent. Mutating caller exclusions after Open did not alter runtime scope, and reopen retained prior request evidence. Invalid limits/paths were rejected before creating session/checkpoint resources. Full embedded test selection under race and SDK vet passed using a local HTTP provider. Built CLI custom-option journeys and final qualification remain pending. Evidence: sdk-checkpoint-options-integration.json.

## Built CLI custom checkpoint scope

Permanent RPC fixture now accepts --checkpoint-scope, passing explicit exclusions and capture/store bounds to all three clients. Combined custom-scope/restore journey passed: snapshots retain the configured exclusion and omission, excluded payload exceeds the cap yet is never captured, private source remains unchanged, and confirmed restore/restart replay preserves later user edits. Five local provider calls and two cancellations; raw fixture and hashes archived in checkpoint-options-rpc.json. Final qualification remains pending.

## Application-owned verification evidence

Added explicitly authorised verification command execution under application ownership and the same background guard as checkpoints. It binds command/profile, capture limits and configured execution boundary into a digest and uses the existing host/container backend without container-to-host fallback. Status recaptures current state and checks referenced evidence. A real command regression verifies busy rejection, stdout/exit recording, passed-to-stale after an edit and unverified after snapshot deletion. Targeted race test and vet passed; final guard refinement was re-tested. User-facing profile/confirmation, persistence wiring and final qualification remain pending. Evidence: app-verification-integration.json.

## Bounded explicit verification profiles

Added strict JSON verification configuration with named literal argv profiles and evidence directory, at most 64 KiB and 32 profiles. Relative evidence directories resolve beside the config file. Validation rejects duplicate/invalid names, unknown fields, trailing JSON and invalid/oversized argv; returned profiles deep-copy command slices. Reading does not create resources or execute commands and opens nonblocking before regular-file validation. Shared profile type adopted by app. Targeted config/app race tests passed; an unkeyed shared-type literal caught by vet was corrected, with original output retained and final vet passing. Startup/configuration and user-facing execution wiring remain pending. Evidence: verification-config-integration.json.

## Confirmed named verification and saved assessment

Controller configuration now validates and copies named profiles and requires an existing private evidence directory outside the workspace. Profile views include exact command, execution boundary and confirmation digest. Named execution rejects mismatched confirmation, holds application/background ownership through command and bounded evidence publication, and returns record ID plus current assessment. Saved assessment reloads integrity-checked records and referenced snapshots/output against current workspace/profile. Real-command regression verifies persisted passed evidence, immutable views, wrong-digest refusal and stale saved status after later edits. Targeted race test and vet passed. Startup/RPC/TUI wiring and final qualification remain pending. Evidence: verification-profiles-integration.json.

## Verification startup and read-only RPC

CLI accepts explicit --verification-config for interactive/RPC modes with checkpoints; the SDK accepts a copied VerificationOptions configuration and requires checkpoint storage. Controller configuration validates the existing private external evidence directory. RPC verification.profiles exposes exact commands/digests/boundaries; verification.check reassesses saved records. The embedded real-runtime test now verifies profile discovery alongside checkpoint capture and session reopen. Targeted SDK/RPC/app race tests and CLI/SDK/RPC/app vet passed. Async confirmed execution, terminal controls and built verification journeys remain pending. Evidence: verification-startup-integration.json.

## Asynchronous confirmed RPC verification

Added verification.run with confirmed profile/digest, durable control intent before dispatch, immediate request record, background execution and persisted completion retrievable via request.get. The normal cancel command also cancels verification; dispatcher close cancels and joins its worker. Duplicate IDs return existing ledger records without re-executing. Tests run real successful/cancellable shell commands, observe startup before cancellation, wait for persisted completion and verify duplicate result identity. Targeted race suite and vet passed. Disconnect/crash/built-client qualification and terminal controls remain pending. Evidence: verification-async-integration.json.

## Verification disconnect join and durable replay

Extended real-command RPC regression with explicit disconnect, bounded five-second dispatcher join and kernel process-existence checks after cancellation/disconnect completion. Success, cancelled and disconnected results are replayed through a new dispatcher after ledger close/reopen, preserving the exact original completion. Three cases passed 20 repetitions under race; targeted suite and vet passed. This is macOS development evidence, not native Linux/process-kill or final cancellation performance qualification. Built CLI journeys and terminal verification remain pending. Evidence: verification-disconnect-integration.json.

## Built CLI verification journey

Added a permanent standalone RPC runner with no provider calls. Three real CLI processes load explicit profiles, reject missing confirmation, execute passing/failing commands, verify content-addressed saved evidence, reassess stale after a user edit, replay the original result after restart, and cancel/join hanging commands on cancel and disconnect. Kernel process checks confirm each command is gone after completion. Raw clients, evidence records, config and hashes are archived in verification-rpc.json. Terminal verification and final qualification remain pending.

## Terminal verification review, confirmation and recheck

Added /verify [profile], /verify-confirm PROFILE DIGEST and /verify-check PROFILE EVIDENCE-ID. Exact argv and execution boundary are displayed before single-use confirmation. Commands and evidence checks use the cancellable joined session-operation worker; result/error presentation preserves evidence IDs and bounds inline output. Status remains distinct from exit code, including stale results with exit zero. Targeted verification/restore/recovery TUI race tests and vet passed. Built terminal verification and full qualification remain pending. Evidence: tui-verification-integration.json.

## Built terminal verification and stale reassessment

Permanent 140x45 PTY runner starts a fresh CLI with explicit verification configuration, reviews the command, verifies no evidence was created before confirmation, confirms the displayed digest and observes passed/exit-zero plus saved evidence ID. It verifies the record hash, edits source, invokes /verify-check and observes stale/exit-zero before clean quit. Zero provider calls. Raw terminal, fixture and hashes archived in verification-terminal.json. Retention and final qualification remain pending.

## Bounded verification evidence retention

Save now uses default retention of 1000 records/256 MiB, with an explicit bounded SaveWithLimits API. A private nonblocking exclusive lock serialises writers. Inventory scans are bounded, reject unsafe entries and count orphan/unknown regular-file bytes; no accepted evidence is automatically evicted. Existing valid identical records remain saveable at capacity and directory durability is reconfirmed. Tests verify record/byte refusal, prior evidence survival, idempotent save and concurrent writer exclusion. Full verification race suite passed; vet passed before a final directory-sync refinement, which was re-tested. Configuration wiring and retention cleanup/fault qualification remain pending. Evidence: verification-retention-integration.json.

## Configured verification capacity and failure reporting

Verification configuration now accepts max_records/max_bytes with validated zero-default values and passes them to evidence publication. A real application regression fills a one-record store, runs a second successful command, verifies persistence rejection yields no evidence ID and an unverified assessment, and confirms the prior record remains readable. Configuration defaults/custom/rejected bounds and RPC lifecycle regressions passed under race; vet passed. This does not reserve space before execution, so commands may run when final evidence cannot fit; the outcome explicitly reports that limitation. Cleanup/fault and final qualification remain pending. Evidence: verification-capacity-integration.json.

## Verification catalogue and explicit deletion

Added bounded sorted 32-ID evidence pages without status claims. Explicit deletion takes the writer lock, loads and verifies the exact record, checks workspace identity, removes only that record and syncs the directory; uncertain durability is reported. Referenced snapshots/output and other records remain untouched. Tests verify listing/order/offsets, workspace mismatch refusal, selected deletion, remaining record/snapshot survival and preservation of corrupt evidence on refusal. Full verification race suite and vet passed. Application/RPC/TUI cleanup wiring and fault qualification remain pending. Evidence: verification-catalogue-integration.json.

## Application and RPC selected evidence cleanup

Added owned evidence catalogue/deletion methods plus verification.list and confirmed verification.delete RPC operations. Deletion uses the durable control ledger for duplicate-request replay. Real verification fixture tests discover the saved ID, reject missing confirmation without mutation, delete the selected record, replay the deletion response and verify referenced snapshots survive while saved assessment correctly becomes unavailable. Targeted app/RPC race suite and vet passed. Terminal cleanup and built/fault qualification remain pending. Evidence: verification-cleanup-integration.json.

## Terminal evidence retention controls

Added /verify-list [offset] and /verify-delete LISTED-ID confirm, requiring a displayed ID and consuming selection before the joined worker. Tests verify pagination guidance, missing/wrong confirmation, unknown IDs, active-run rejection and consumed selections. Targeted race suite and vet passed. Extended real PTY journey passes review, command confirmation, stale reassessment, listing and confirmed deletion, preserving snapshots and clean exit with zero provider calls. Built evidence is in evidence-terminal.json. Final qualification remains pending. Evidence: tui-evidence-integration.json.

## Bounded personal and ancestor instructions

M6.1 started. Startup discovery now loads personal ~/.hand guidance and ancestor guidance broadest to nearest, retaining HAND.md precedence at each level and reporting shadowed AGENTS.md paths. Structured sources preserve path/body provenance. Reads exclude symlinks/nonregular files, cap each body at 32 KiB and total bodies at 128 KiB, and bound ancestry at 64 levels. Diagnostics expose exclusions instead of silently falling back. Prompt explicitly preserves user-request precedence and states that guidance grants no capabilities. Targeted race tests passed after correcting a duplicate-function editing error; initial build failure is retained. Vet passed. Lazy nested guidance, skills migration/resource bases/refresh, context inspection and final qualification remain outstanding. Evidence: instructions-integration.json.

## Conventional skill discovery and resource provenance

Added project and personal .agents/skills discovery after existing project and personal .hand/skills stores, preserving existing winners during migration. Index includes source paths and visible collision/exclusion diagnostics. Skill bodies include their resource base directory and retain normal execution permissions. Discovery is bounded to 256 entries per store and 32 KiB per body; unsafe leaf reads fail closed. Tests cover conventional locations, project conventional precedence, Hand migration precedence, resource directory, provider reindex after self-authoring, and oversized rejection. Targeted race suite and vet passed. Harness still caches the index at startup, so runtime refresh/reload is not yet implemented or accepted. Evidence: skills-discovery-integration.json.

## Harness skill refresh at safe boundaries

The development Harness now provides RefreshSkills guarded by runtime run ownership, and refreshes skill context before each model request after prior tool execution has joined. Prompt reconstruction uses current skill discovery instead of the startup-captured index, including on MCP catalogue refresh. A real disk-store test verifies newly authored and removed skills, busy rejection without mutation, and preservation of identity/configuration/memory. Targeted runtime race tests and vet passed. Full model-request self-authoring, Hand reload controls and final qualification remain pending. Harness is now dirty relative to 5998569; the incremental patch and exact source hashes are recorded in harness-skills-refresh.json. No release occurred.

## Runtime self-authoring request regression

Added a deterministic three-request provider fixture driving the actual skill_manage tool to create a disk-backed skill, then load_skill to retrieve its procedure. Assertions inspect the system prompt sent to the second and third model requests, verify the new description was absent initially, preserve fixed identity, check the loaded body and on-disk persistence, and require terminal success. Targeted race tests and vet passed after fixing an incorrect event constant in the new test; the original build failure is retained. This uses no live provider or paid calls. Hand reload and final acceptance remain pending. The cumulative Harness delta from 5998569 is harness-skills-authoring.patch, with hashes and evidence in harness-skills-authoring.json.

## Explicit skill reload controls

Added application-owned ReloadSkills, terminal /reload via the cancellable joined operation worker, and negotiated skills.reload RPC backed by the durable control ledger. Reload changes the skill index only and grants no permissions. Tests verify busy/cancelled refusal, authored skill discovery, terminal argument/busy guards and worker completion, RPC strict parameters, duplicate replay without another refresh, and fresh-ID refresh after skill removal. Three targeted tests pass under race after correcting missing hello and service ownership in the RPC fixture; original failure retained. Vet passed before the final test-only fixture correction. The staged Hand now requires the dirty Harness delta in harness-skills-authoring.patch; primary released dependency is unchanged. Built journeys and final qualification remain pending. Evidence: skills-reload-integration.json.

## Lazy nested guidance on file reads

Successful file reads now discover only applicable directories below the workspace, broadest to nearest, using the bounded instruction reader and HAND.md collision precedence. Guidance precedes returned file contents, and structured source/diagnostic metadata is retained. Failed reads load no guidance. The normal registry and isolated worker both use the wrapper, retaining exact-path semantics inside the worker and normal host resolution outside. Tests verify nested precedence, source provenance, sibling/root exclusion, outside-path exclusion, refreshed discovery after a new instruction file, failed-read behavior, and both path modes. Twelve targeted race tests and vet passed. Rebuilt worker journeys, write/edit/search behavior, large-result/context qualification and final acceptance remain pending. Evidence: nested-guidance-integration.json.

## Built worker guidance and skill resource journey

Fixed worker load_skill to use the shared provider, enabling conventional skill discovery and resource provenance inside its boundary. A permanent native-worker runner executes four real processes: nested guidance with metadata and scope checks, conventional skill loading with resource base, resource-file read, and fresh guidance after a file update. All passed with zero provider calls. Full toolworker race suite and vet passed. Raw requests/responses, fixture and hashes are in guidance-worker-native.json; cumulative implementation is guidance-worker-integration.patch. Added guidance-usage.md documenting precedence, bounds, resource resolution, reload and remaining limitations. Container, mutation/search and final qualification remain pending.

## Runtime context inspection foundation

M6.2 started. Added idle-owned runtime context inspection of installed static prompt, effective conversation, tool results and registered schemas. Uses effective post-compaction history and the existing image resolver/estimator, reports counts and estimated tokens without returning bodies, and clearly labels estimates as distinct from provider usage. Explicit limitations cover future input, dynamic suffix/retrieval, image allowances and component rounding. Tests verify the selected payload, counts, preserved history and busy/cancelled rejection alongside skill refresh regressions. Three targeted race tests and vet passed. Source-level instruction/skill decomposition, application interfaces, pinned constraints and all remaining M6.2 acceptance work are pending. Cumulative Harness delta and evidence: harness-context-inspect.patch/json.

## Context inspection application controls

Added application-owned context inspection, /context through the joined session operation worker, and negotiated read-only context.inspect RPC with strict empty parameters. Terminal output includes estimated total, category counts/contributions, estimation method and explicit exclusions. Tests verify detached report state, busy rejection, terminal argument/active-run guards and completion, RPC estimate disclosure and no implicit skill refresh. Three targeted race tests and vet passed. Source-level detail, built journeys, pins/compaction and final qualification remain pending. Cumulative Hand integration requires harness-context-inspect.patch. Evidence: context-controls-integration.json.

## Built RPC context and reload journey

Permanent built-CLI runner passes strict context parameters, estimate disclosure, unchanged installed prompt before explicit reload, added skill contribution, duplicate reload replay without another refresh, removal after a fresh reload, and clean exit. Observed estimated totals are 2074 initially, 2290 after adding/reloading a conventional skill, and 2074 after removal/reload. No model runs requested. Raw transcript, fixture, binary and runner hashes are archived in context-rpc.json. Added context-usage.md describing estimate scope and incomplete source-level/compaction work. Terminal journey and final qualification remain pending.

## Installed context source snapshots

Harness accepts caller-provided system prompt source metadata and an optional atomic skill index/source snapshot interface. Context inspection returns copied installed sources, capped at 128 with an omitted count, and labels source estimates as subsets rather than additional tokens. Skill refresh replaces its source snapshot with the index. Targeted runtime race tests and vet passed; Hand source snapshot integration has separate evidence. Source details for nested tool-result guidance and large artifacts remain pending. Cumulative Harness delta: harness-context-sources.patch.

## Hand instruction and skill source inspection

Agent construction captures instruction paths and body estimates with the actual prompt; skill provider returns index and source estimates in one discovery pass. Terminal context output lists installed sources with subset wording and omitted count. The integration regression edits instruction files after startup, mutates caller/returned metadata, authors a skill and reloads; installed instruction metadata stays accurate and detached while skill sources update on refresh. Targeted Hand race tests and vet passed. Requires cumulative harness-context-sources.patch. Built source views, nested-result/artifact detail and final acceptance remain pending. Evidence: context-sources-integration.json.

## Durable objectives and constraints

Harness now supports an idle-owned session-wide context pin set, persisted as versioned annotations and independently reinserted into each model request. Up to 16 uniquely identified objective/constraint pins of 2048 UTF-8 bytes each; invalid input is rejected before persistence, an empty set clears, unsupported stored records fail visibly. Pins remain subject to current user requests and do not change permissions. Inspection counts pins separately. A real disk-session fixture applies three direct compactions with summaries omitting the pins, reopens the session during the sequence, and verifies both pin texts in all three scripted-provider requests. Five targeted race tests and vet passed. Application controls, live summariser/structured-summary qualification and final acceptance remain pending. Cumulative Harness delta/evidence: harness-context-pins.patch/json.

## Application and RPC pin controls

Added owned pin listing/upsert/removal, terminal /pins, /pin ID objective|constraint TEXT, and /unpin ID through joined workers. RPC context.pins is read-only; context.pin/unpin use durable control replay. Tests verify replacement preserves other pins, returned state is detached, invalid/missing updates preserve state, busy rejection, terminal argument guards/worker completion, and replaying a removed pin creation does not recreate it. Five targeted race tests and vet passed. Built journeys and summariser integration remain pending. Cumulative Hand delta requires harness-context-pins.patch. Evidence: pins-controls-integration.json.

## Built pin persistence and committed mutation results

Pin mutations now return the committed update/removal directly, avoiding a second read that could report failure after successful persistence. Listing remains explicit. Three targeted race tests and vet passed. A permanent built RPC runner uses two CLI processes: create/select session, persist objective and constraint, reject invalid update, inspect pin contribution, restart/select the same session, verify pins, remove one, replay its old creation without recreating it, and remove the remainder. Both exits clean; no model runs requested. Raw transcripts/session fixture and hashes are in pins-rpc.json. Terminal and summariser qualification remain pending. Cumulative implementation: pin-results-integration.patch.

## Preserve history when summarisation fails

A new regression failed against the existing placeholder fallback: failed summarisation reported Compacted and replaced active history with a stub. Removed that fallback. Exhausted summarisation now returns its error; the manager reports a skipped pass, increments failure accounting and leaves effective view, raw entries and selected leaf unchanged. Existing successful bounded fallback remains. Updated prior tests that explicitly required placeholder compaction to assert failure preservation instead. Full compaction race suite, targeted runtime compaction/pin regressions and vet passed. Before-fix failure and all raw outputs are retained in harness-summary-preserve.json. Summariser pin/focus integration and final qualification remain pending.

## Pinned guidance in summariser requests

Added bounded immutable compaction guidance carried by context, combined with per-call focus by the manager. Runtime admission captures pins for automatic/reactive/async compaction; idle CompactionContext supports manual callers. No shared manager configuration mutation. A scripted-provider regression verifies actual manual and async summariser requests both include the pin, manual focus reaches its request, and that one-off focus does not leak to later automatic compaction. Four targeted runtime/compaction race tests and vet passed. Runtime admission-specific and built manual focus qualification remain pending. Cumulative Harness evidence: harness-compaction-guidance.patch/json.

## Manual compaction focus forwarding

Added CompactWithFocus while preserving the existing Compact API. Manual compaction binds current pins, validates focus to 4096 UTF-8 bytes without NUL, and forwards it to the manager. /compact now passes trailing text instead of discarding it; the existing cancellable/joined worker remains. Targeted app/TUI race tests and vet passed after correcting an initial misplaced edit; failure retained. Requires harness-compaction-guidance.patch. Built focus journeys, RPC equivalent and separate summariser profiles remain pending. Evidence: compaction-guidance-integration.json.

## Bounded summariser generation

Added MaxOutputTokens with default 4096 and validated 1-32768 range, forwarded into the actual summariser request. Invalid limits and unavailable providers fail before dispatch. Response accumulation is capped at 256 KiB; oversized output returns an error rather than a summary and follows history-preserving failure handling. Tests verify default/custom/boundary/invalid limits and oversized refusal without retry. Full compaction race suite and runtime/compaction vet passed. Profile configuration and financial admission budgets remain pending; an output token cap is not a cost guarantee. Evidence: harness-summary-budget.patch/json.

## Application summariser request regression

A real Controller.CompactWithFocus fixture verifies that an oversized focus is rejected before dispatch, and a successful manual request uses the configured summary model, 1024 output-token cap, pinned constraint and user focus. Pins remain available after compaction. Two targeted application race tests and vet passed. Requires harness-summary-budget.patch. Built/provider profile configuration and final acceptance remain pending. Evidence: summary-budget-integration.json.

## Independent summariser profile selection

Added application review and selection of a named profile for summarisation. Review discloses provider/model/destination, credential environment reference, per-attempt output/timeout limits, history/pin data transfer and retry usage. Selection validates a matching full-configuration digest before provider construction and commits only after successful construction and cancellation checks. Independent selection survives main-model and main-profile switches; failure and busy cases preserve prior configuration. Targeted app race tests and vet passed, with no provider calls. Terminal/RPC/startup selection, return-to-main mode and full qualification remain pending. Evidence: summary-profile-integration.json.

## Summariser review controls

Added /summarizer [PROFILE [OUTPUT_TOKENS TIMEOUT_SECONDS]] and single-use /summarizer-confirm DIGEST with destination, credential reference, per-attempt limits and history-transfer disclosure. Review state is cleared by session operations and consumed before selection. RPC summarizer.review/status are read-only; summarizer.select requires explicit confirmation plus matching digest and uses durable control replay. Tests verify disclosure, invalid arguments, missing review, active-run rejection, consumed review after failure, RPC unconfirmed refusal, one provider construction and duplicate replay, and selected status. Three targeted race tests and vet passed. Selection remains process-local; persistence, return-to-main mode and built journeys remain pending. Evidence: summary-controls-integration.json.

## Reviewed return to main-model summarisation

SummarizerOptions now supports follow_main without a separate profile. Review binds the current main route and limits; selection rejects stale reviews after main-model changes, reuses the current main client without constructing another provider, and restores subsequent main-model following while retaining reviewed generation limits. Status reports effective following configuration. /summarizer-follow reviews this mode and uses the existing single-use confirmation; RPC uses follow_main in the same reviewed options. Targeted summariser race tests and vet passed. Startup persistence and built mode-switch journeys remain pending. Evidence: summary-follow-integration.json.

## Integrated development validation after context changes

Rebuilt the Linux ARM64 worker from current staged source and ran all Hand packages uncached under the race detector with cached Docker fixtures enabled. 920 tests/subtests passed, zero failed or skipped tests; four command/example packages contain no tests and are listed separately. Harness runtime and compaction suites passed 346 tests/subtests, zero failures/skips. Full Hand vet and scoped Harness vet passed. No source changes were made during the runs. Archived raw logs, dependency files, worker hash and post-run source snapshots in m6-full.json. This remains dirty development evidence, not final coverage/platform/performance/live/candidate qualification. Next implementation work remains summariser persistence and the remaining M6–M8 requirements.

## Reviewed summariser startup configuration

Added --summarizer-config for interactive/RPC startup. Strict versioned JSON carries reviewed options and digest without credential values; bounded regular-file loading refuses symlinks, unknown fields, trailing content, invalid routes and limits. Startup revalidates current profile configuration before constructing a summariser provider. Permanent regression serialises a real review, reads it from disk, restores its route and limits in a fresh controller, then changes the destination and verifies refusal before provider construction or configuration mutation. Targeted app/CLI race tests and vet passed. Built CLI restart journeys and final qualification remain pending. Evidence: summary-startup-integration.json.

## Built summariser selection and startup journey

Permanent RPC runner exercises actual CLI review, confirmed selection and duplicate replay, writes the reviewed startup file, restarts and compares the complete restored selection, then switches to reviewed follow-main mode. Changing the profile endpoint causes a third startup to exit 5 without protocol output. Both successful RPC processes exit cleanly. No model runs requested. Built native development evidence and raw transcripts/configuration are archived in summary-rpc.json; binary and runner hashes recorded. Terminal journeys, live summary quality and final candidate qualification remain pending.

## Provider-attempt budget admission boundary

Added chained context-bound admission guards to Harness ObserveChat, the shared boundary for generation, retries/non-streaming fallback and compaction. Guards reserve before dispatch and settle once before terminal delivery. A later denial unwinds prior reservations as not_dispatched; nil usage stays unknown. Settlement failures replace successful terminal events with explicit errors. Permanent race tests cover denial, nested guards, all call categories, concurrent reservations, cancellation and visible settlement failure. Existing scoped runtime/compaction usage regressions and llm vet pass. This is budget enforcement infrastructure, not completed budgeting: persistent token/cost/time ledgers, price provenance, policy, exhaustion/resume controls and integrated qualification remain required. Evidence: harness-admission.patch/json.

## Durable session token reservations

Added session conditional annotation appends and a token admission ledger with durable absolute-ceiling decisions, pre-dispatch reservations, usage reconciliation and latched exhaustion requiring a new explicit decision. Interrupted reservations and missing/partial usage retain conservative charges; known overruns exhaust admission. Concurrent ledger owners on the same Session cannot reserve the same capacity. Records survive disk restart and compaction without affecting conversation state; bounded journal admission leaves settlement capacity. Permanent race tests cover restart, explicit resume, competing owners, uncertainty, overrun and failed journal writes preventing dispatch. Annotation regressions and vet pass after fixing an initial test API compile error, retained in evidence. This is token-only infrastructure: cost/time/per-run budgets, pricing provenance, Hand controls and final qualification remain pending. Evidence: harness-token-ledger.patch/json.

## Runtime token budget enforcement

Runtime Run and manual CompactionContext now attach the persisted session token admission policy when configured. Parent guards remain attached and repeated context preparation does not duplicate the session reservation. Input estimates include effective system parts, messages and tool schemas; the provider output cap is reserved separately. Idle APIs inspect and explicitly decide the absolute ceiling. A scripted actual-runtime regression verifies exhausted admission makes zero provider calls, explicit resume allows generation, and manual compaction records a second request against the same ledger. Targeted race test and vet passed. Cost/time/per-run limits, budget-specific terminal outcomes, immediate cancellation of owned work and final qualification remain pending. Evidence: harness-token-runtime.patch/json.

## Application and RPC token budget controls

Added owned bounded token-budget inspection and explicit ceiling decisions. RPC budget.tokens reports limit, committed tokens including uncertainty, exhaustion, attempt counts and estimation disclosure. budget.tokens.decide requires confirmation and durable control replay. Regression verifies no unconfirmed change, later explicit decisions are not overwritten by replay of an old request, bounded status and invalid ceiling refusal. Targeted race test and vet passed. Built journeys, terminal controls and remaining budget dimensions remain pending. Requires harness-token-runtime.patch. Evidence: token-controls-integration.json.

## Exhaustion on token settlement

After durable settlement, an exhausted token ledger now returns ErrExhausted, so ObserveChat delivers an error instead of a successful terminal event. This covers known overruns and exhaustion latched by concurrent admission; charges remain persisted and explicit resume is still required. Updated overrun regressions require the exhaustion error, and token ledger/runtime race tests plus vet pass. Cancellation of tools already started while streaming remains pending. Evidence: harness-budget-outcome.patch/json.

## Application budget exhaustion outcome

Wrapped Harness budget exhaustion is now classified as budget_exhausted with session_token_budget reason and exit code 4. Both start errors and runtime terminal failures receive this classification. Service regressions verify exactly one unverified terminal event and no completion validator invocation. Scoped application outcome race tests and vet pass. Requires harness-budget-outcome.patch; built CLI persistence/exhaustion journey and cancellation of existing owned work remain pending. Evidence: budget-outcome-integration.json.

## Built session token budget journey

Actual CLI RPC runner sets a one-token ceiling, verifies budget_exhausted without any provider request, restarts/selects the same session and verifies continued exhaustion, explicitly resumes to 100000 tokens, replays an older decision without reducing the new ceiling, completes one local scripted provider request and records exactly 15 reported tokens. A third process verifies that charge persists. All three processes exit cleanly. Initial sandbox server bind was denied; approved loopback execution passed. Raw provider requests, RPC transcripts, sessions and binary/runner hashes archived in budget-rpc.json. No paid calls. Remaining budget dimensions and final qualification remain pending.

## Terminal session token budget controls

Added /budget inspection and /budget-tokens TOTAL as an explicit absolute-ceiling decision through the joined session worker. Status discloses prior/reserved token charges, uncertain attempts, exhaustion and estimation limits; help distinguishes a total ceiling from added allowance. Regression exercises command parsing, invalid/overflow limits, active-run guard, committed decision, status rendering and worker release. Targeted race test and vet passed. Actual PTY journey, other budget dimensions and final qualification remain pending. Evidence: token-terminal-integration.json.

## Explicit pricing snapshots and conservative quotes

Added route/source/version/currency/validity-tagged pricing snapshots, integer nano-currency quotes, conservative reservation using the maximum input/cache rate, and cache-aware reported-usage reconciliation. Strict quoting rejects missing, expired/future or unbounded tariffs; advisory quoting returns known=false with a reason. Invalid tariffs/counters and arithmetic overflow fail visibly. Missing usage is unknown, while an explicitly bounded zero tariff can be known-free. Tests verify subset cache accounting, exact integer rounding, snapshot detachment, uncertainty and overflow; full budget race suite and vet passed. Rates are labelled test fixtures, not market prices. Durable monetary ledger, route binding, price configuration and remaining budget dimensions remain pending. Evidence: harness-pricing.patch/json.

## Durable monetary reservations and uncertainty

Added session cost ledger with one immutable currency, explicit strict/advisory absolute-ceiling decisions, pre-dispatch atomic reservations, copied per-attempt tariff/source/version snapshots and conservative settlement. Unknown advisory prices remain explicit and prevent silently enabling strict mode later; strict unknown pricing blocks dispatch. Interrupted reservations survive exclusive disk-session restart; competing owners cannot spend the same balance. Known overruns latch exhaustion, explicit resume retains charges, and unrepresentable settlements retain reservations without corrupting the journal. Full budget race suite and vet passed after correcting a local overflow-check compile error, retained in evidence. Monetary route binding/configuration and Hand controls, time/per-run budgets, owned-work cancellation and final qualification remain pending. Evidence: harness-cost.patch/json.

## Provider destination binding for cost admission

ChatRequest now carries local-only route metadata; Runtime generation/run-turn and independent Summarizer requests populate it from their client route. Cost admission rejects tariffs whose provider or destination differs from that metadata, in addition to checking the model. A regression verifies missing metadata and same-model/different-endpoint or provider mismatches cannot reserve or dispatch. Scoped cost/runtime race tests and budget/runtime/compaction/llm vet passed. Application configuration must install metadata atomically with provider clients. Full monetary configuration/enforcement in Hand and final qualification remain pending. Evidence: harness-price-route.patch/json.

## Hand client route metadata

Profile configuration and successful model/profile switches install runtime route metadata using provider identity and explicit endpoint or labelled provider default. Independent summariser selection installs its own route; follow-main mode and subsequent main switches keep the summariser aligned, while independent selections retain their destination. Extended actual controller summariser regressions check independent and following route identity; scoped profile/summariser race tests and vet passed. Monetary policy configuration, attachment and built price-route journeys remain pending. Requires harness-price-route.patch. Evidence: price-route-integration.json.

## Runtime monetary policy enforcement and RunTurn bypass fix

Added bounded versioned session tariff tables, idle-owned explicit price replacement and cost-ceiling APIs. Runtime Run, RunTurn and manual CompactionContext attach persisted token and cost guards; runtime resolution uses exact provider/destination/model and includes effective system/tool/message input estimates. Strict unpriced generation is refused; separate summariser requests select their own route tariff. Permanent regressions verify zero unpriced dispatches, generation and compaction accounting, durable price/ceiling restart, duplicate/busy replacement refusal and detached metadata. Inspection found RunTurn did not attach existing token limits; the shared budget path fixes that bypass and a regression verifies zero denied dispatches. Final scoped race tests and vet passed. Hand monetary controls/configuration and dedicated cost outcomes, time/per-run budgets, cancellation and final qualification remain pending. Evidence: harness-cost-runtime.patch/json.

## Typed monetary exhaustion cause

Cost ledger exhaustion now returns a dedicated ErrCostExhausted wrapping the shared ErrExhausted classification. Callers can report cost versus token exhaustion without string matching. Scoped cost-ledger race tests and vet passed. Evidence: harness-cost-controls.patch/json.

## Monetary application and RPC controls

Added idle-owned monetary status, explicit ceiling decisions and tariff table read/replacement. Bounded status includes currency, nano-unit ceiling/committed/compaction amounts, uncertainty and billing-limit disclosure. RPC budget.cost.decide requires confirmation and explicit strict boolean; budget.prices.set requires confirmation and shares durable control replay. Regressions verify exact refusal codes, immutable currency, old-decision replay preserving newer settings, and replayed tariff installation not overwriting later clearing. Outcome tests distinguish session_token_budget, session_cost_budget and cost_price_unknown for both startup and runtime errors, with no verification pass. Final scoped race tests and vet passed. Built monetary journeys, terminal configuration and remaining budget dimensions remain pending. Requires harness-cost-controls.patch. Evidence: cost-controls-integration.json.

## Built monetary-budget and combined admission journey

Actual CLI runner enables token and strict monetary ceilings, verifies unknown pricing blocks dispatch and releases the prior token reservation, installs an explicit local tariff, exhausts a tiny monetary ceiling without dispatch, then restarts and verifies prices/ceiling/exhaustion persist. Explicit resume permits exactly one scripted request; replay of the old ceiling does not overwrite it. Reported 15 tokens reconcile to 15 nano-units under the fixture tariff. A third process confirms monetary charges and price provenance persist. Three clean exits, zero paid calls. Raw provider requests, transcripts, session data and binary/runner hashes archived in cost-rpc.json. Remaining budget dimensions and final qualification remain pending.

## Decimal monetary controls in terminal

Added /cost [CURRENCY TOTAL strict|advisory] and /prices via joined session workers. Monetary input uses integer decimal parsing with up to nine places, refusing exponent/negative/overflow/excess-precision values; formatting preserves nano-unit amounts without floating point. Status shows absolute/committed/compaction amounts, uncertainty and billing limitations; tariff inspection shows sanitized route, rates, source/version, validity and bounded-charge assertions. Tests cover decimal boundaries, invalid input, active-run refusal, committed exact ceiling, status and worker release. Targeted race tests and vet passed. Terminal tariff-file review/import and actual PTY journeys remain pending, as do remaining budget dimensions and final qualification. Evidence: cost-terminal-integration.json.

## Terminal tariff-file review and exact confirmation

Added /prices-review PATH and /prices-confirm DIGEST. The loader accepts strict version-1 JSON with an explicit prices array, rejects unknown/trailing/missing/null input, nonregular/symlink files and data above 48 KiB, and validates at most 16 unique route tariffs. The UI displays the full tariff/provenance list and replacement count; confirmation is single-use and applies the reviewed content rather than rereading a potentially changed file. Session operations clear pending review. Permanent tests verify strict loading, file changes after review, exact installed destination, active-run and repeated-confirmation refusal, and digest mismatch after content mutation. Targeted race tests and vet passed. Actual PTY qualification, time/per-run budgets, cancellation and remaining goal requirements remain pending. Evidence: price-review-integration.json.

## Persistent session deadlines and cancellation

Added explicit positive wall-clock allowances up to 30 days as durable session deadlines. Idle time counts; restart preserves expiry; only a new explicit decision renews it. Runtime Run and RunTurn bind owned contexts, cancel after child joins, and refuse expired starts. Provider error paths retain cancellation causes; time expiry emits a distinct ErrTimeExhausted rather than a generic abort. Permanent tests verify actual stream cancellation and termination, restart/explicit resume, validation and distinction from parent cancellation. Scoped budget/runtime/observer race tests and vet passed. SessionDeadlineContext supports owned manual operations with explicit cleanup. User controls, broader process/tool cancellation tests, per-run limits and final qualification remain pending. Evidence: harness-deadline.patch/json.

## Time-budget outcome and manual compaction binding

Hand classifies time expiry as budget_exhausted with session_time_budget reason. Outcome regressions cover startup and streamed failure paths without completion validation. Manual compaction now prepares the owned deadline context, joins its synchronous operation before timer cleanup, and returns its cancellation cause with usage/persistence errors. Scoped app compaction/outcome race tests and vet passed. Time controls and built deadline journeys remain pending. Requires harness-deadline.patch. Evidence: deadline-integration.json.

## Owned time allowance and RPC controls

Added bounded time-budget status and explicit seconds-based allowance decisions through the application owner. RPC budget.time reports configured/decision/deadline/expired/remaining state and idle-time disclosure; budget.time.decide requires confirmation, validates 1-2592000 seconds before duration conversion, and uses durable replay. Regression verifies no implicit default, confirmation refusal, new deadline installation, replay preserving the newer decision and overflow refusal. Scoped time/outcome race tests and vet passed. Terminal controls, built expiry/cancellation journeys, per-run limits and final qualification remain pending. Evidence: time-controls-integration.json.

## Built deadline cancellation and renewal journey

Actual CLI runner configures a two-second allowance and opens a local provider heartbeat stream. Expiry returns an idle, unverified budget_exhausted/session_time_budget terminal in about 2.01 seconds, and the fixture observes disconnection. Unknown usage remains recorded. Restart/select preserves the exact expired deadline; replay cannot renew it and a subsequent expired prompt makes no provider request. A new explicit 60-second decision permits one completed local response. Two clean exits, two local requests, zero paid calls. Raw transcripts, provider requests, session data and binary/runner hashes archived in time-rpc.json. This is one development timing observation, not the repeated performance gate; tool/process cancellation and final qualification remain pending.

## Terminal wall-clock allowance controls

Added /time-budget inspection and /time-budget SECONDS explicit renewal through the joined session worker. Display includes absolute UTC deadline, remaining time, expiry, idle-time semantics and preservation of earlier token/cost charges. Regression verifies invalid/overflow allowance refusal, active-run protection, explicit decision, read-only inspection preserving the deadline, expired guidance and worker release. Targeted race test and vet passed. Actual PTY budget journeys, tool/process expiry, per-run limits and final qualification remain pending. Evidence: time-terminal-integration.json.

## Deadline tool cancellation cause and durable pairing

Deadline cancellation now stores the actual cancellation cause in aborted tool results and knowledge-graph messages; ordinary user cancellation retains its existing label. Added a backward-compatible session constructor for aborted results with a reason. A real BashTool regression exercises both streaming-disabled and streaming-enabled configurations (bash remains sequential because it declares itself unsafe for concurrency), waits beyond a descendant scheduled write to prove process-group cleanup, reopens the exclusive disk session, and requires exactly one call/result pair with Aborted, IsError and the time-exhaustion cause. Scoped uncached race tests and runtime/session vet passed. This is native macOS development evidence, not repeated performance or final candidate qualification. Evidence: harness-time-process.patch/json.

## Concurrent streaming-tool budget termination

A permanent regression demonstrated that provider errors after concurrent tool start could wait indefinitely without cancelling the tool and discard its session pair. Runtime now owns a cancellation-cause context; terminal errors with started tools cancel owned work and use normal result reconciliation instead of draining/discarding results or replaying potentially executed effects. Budget errors stop retry admission. Tests require tool start before provider termination, then verify joining, original terminal cause, reopened call/result pairs, and durable actual token/cost overrun charges. Real one-second deadlines, real token and cost ledger settlement errors, and ordinary provider failures pass. The failed pre-fix run is retained as regression evidence; subsequent scoped race suites and vet passed. Mixed already-completed/running tool reconciliation still needs audit, as do per-run budgets and final candidate qualification. Evidence: harness-stream-budget.patch/json.

## Ordered interrupted-stream result reconciliation

Replaced early-abort nested channel draining with one ordered pass. Every started tool result is consumed once; completed output keeps its actual result, while aborted and unstarted calls get paired cancellation records. Result persistence uses a context independent of run cancellation so completed output and image references are retained; persistence errors are reported after all started tools join. This fixes completed results being relabelled as aborted, missing pending pairs, and a potential second receive on an already-consumed result channel. Regression tests exercise running-first and completed-first ordering, an all-completed stream failure, exclusive session reopen, original terminal cause, and non-execution of pending tools. Scoped uncached race tests and vet passed; pre-fix failing evidence retained. Development only: duplicate provider IDs, per-run limits and final qualification remain open. Evidence: harness-stream-reconcile.patch/json.

## Provider tool-call identity validation

Run and RunTurn now reject empty and duplicate completed tool-call IDs within each provider response before dispatch can overwrite a result channel or execute an ambiguous call. Streaming rejection cancels and reconciles prior calls. A race detected in the first fix showed usage settlement could emit after runtime event closure; rejection now drains the cancelled observer stream before closing runtime events or returning a turn result. Permanent tests cover streaming, sequential and RunTurn paths for empty/duplicate IDs, execution counts and persisted identity uniqueness. Twenty uncached race repetitions and the final adjacent streaming/RunTurn/deadline suite passed, as did vet. Both the original failing regression and the intermediate race are retained as development evidence. Evidence: harness-tool-identity.patch/json. Final acceptance and per-run budgets remain open.

## Durable per-run ledger primitives

Added separately namespaced run token/cost ledgers and deadlines using the existing conditional-append reservation and pricing semantics. Strict bounded ASCII run IDs distinguish logical operations; reopening an ID preserves charges and deadlines and never grants a new allowance. Session ledger formats remain unchanged. Tests compose session and run admission guards through ObserveChat, prove a run denial releases the session reservation before any provider dispatch, charge generation/retry/compaction in both scopes, reopen durable state, retain unknown interrupted monetary reservations, reject ambiguous IDs, and verify actual deadline expiry plus explicit renewal. Full budget race suite and targeted runtime budget regression suite passed; vet passed. These are primitives only: owner binding to a stable run ID, runtime attachment, user controls and final qualification remain required. Evidence: harness-run-ledger.patch/json.

## Runtime run-budget binding

Added idle runtime APIs for per-run token, monetary and time decisions plus inspection. PrepareRunBudget attaches run guards alongside session/inherited guards, binds both persistent deadlines and rejects unconfigured IDs or replacement of an inherited run ID. Run, RunTurn and compaction retain the prepared context; repeat preparation does not double reserve. Newly enabled dimensions or changed deadlines invalidate stale contexts before execution; existing ceilings are reloaded by ledger guards. Tests verify shared run/session charges across Run, RunTurn and manual compaction, both scopes blocking before provider dispatch, actual deadline cancellation, run monetary exhaustion and explicit resume, and rejection of stale contexts. Scoped uncached race tests and vet passed. Hand still needs owner binding, user controls and specific run-scope outcome labels; this is not end-to-end completion. Evidence: harness-run-binding.patch/json.

## Hand logical-run budget ownership

Added explicit Controller run decisions/selection and persistent selected budget metadata. Service prepares the selected run context once for the entire logical execution, including continuation and completion validators, and joins cleanup before releasing it. New prompts and process restarts retain selection and charges; only explicit decisions change allowances. Run journal records the stable budget ID independently from local event RunID and validates matching start/finish associations. Run token, cost and time exhaustion now have distinct terminal reasons. Tests verify blocked admission, explicit resume, two-turn accounting, durable restart without charge reset, budget-ID journal association, deadline expiry during completion validation, and refusal of an expired subsequent execution. Scoped race tests and vet passed. RPC/TUI controls, bounded run-budget inspection and built journeys remain pending. Evidence: run-owner-integration.patch/json.

## Run-budget RPC controls and bounded inspection

Added budget.run inspection (optional ID, otherwise current selection), budget.run.select, and budget.run.tokens.decide/cost.decide/time.decide. Every mutation requires explicit confirmation; monetary mode is mandatory; malformed fields and overflowing allowances fail. Mutations participate in the durable control ledger so old successful requests cannot overwrite newer ceilings, renew deadlines or switch back to an earlier selection. RunBudgetView exposes bounded token/cost/time counters and disclosures, excluding per-attempt history. Tests exercise confirmation, strict schema validation, missing selection, allowance overflow, inspection and replay semantics; scoped uncached race tests and vet passed. Built CLI and terminal journeys remain pending. Evidence: run-rpc-controls-integration.patch/json.

## Terminal logical-run budget controls

Added /run-budget [ID] inspection plus select, tokens, cost and time subcommands through the joined session-operation worker. Display reports selected/configured scope, absolute ceilings, charges, uncertainty, compaction spending, deadline and persistent-selection semantics. Decisions do not implicitly change selection; monetary input uses exact decimal parsing and explicit strict/advisory mode. Tests verify malformed and overflowing input rejection, precise decimal installation, explicit selection, inspection preserving the deadline, active-run mutation refusal and worker release. Targeted uncached race tests and vet passed. Actual built CLI/PTY journeys and final qualification remain pending. Evidence: run-terminal-integration.patch/json.

## Built CLI run-budget journeys

A fresh native CLI build passed permanent Python-client RPC runners. Token/cost journey: three clean CLI exits; run exhaustion prevents any local provider dispatch, restart preserves selection/exhaustion, explicit resume charges 15 tokens to both session/run and 15 nano-units to the priced run, and old decision/selection replay cannot overwrite newer state. One scripted provider request. Deadline journey: two clean CLI exits; real stream cancelled at a two-second run deadline, provider disconnect observed, expiry persists after restart and replay, expired prompt makes no call, explicit renewal permits completion. Two scripted provider requests; observed expiry 1.992 seconds is not a performance qualification. Zero paid/external provider calls. Binary SHA256 274da2f67683da8ce17a9474947e2b4878da112e117a759377063c040b15329b. Raw transcripts, private fixture session state, runner/client snapshots and hashes archived in run-budget-rpc.json and run-time-rpc.json. Actual PTY and final candidate gates remain open.

## Built terminal run-budget journey

A real 140x45 PTY passed inspection, invalid-token refusal, token/cost/time decisions, explicit selection and selected-run inspection using the same native binary as the RPC journeys. The permanent runner asserts rendered ceilings/deadline and reads persisted annotations after a clean exit: exactly one valid token decision, exact 1.000000001 USD monetary ceiling, one unchanged deadline and one selection. No model prompts or paid calls. Initial runner wait timeout was caused by stopping PTY draining too early after quit; corrected and stronger assertion runs passed, with all attempts retained. Evidence: run-budget-terminal.json. This qualifies the run-budget control journey in development only; broader session-budget/price-review terminal journeys and final candidate gates remain open.

## Broad post-budget regression verification

Current integrated Hand passed its uncached full race suite (968 passes); two SDK tests skipped for a missing Bash-image setting passed in a targeted uncached follow-up (2 passes) using the existing pinned container image. Skips are recorded, not counted as passes. Four command/example packages have no test files. Harness runtime/compaction/budget/llm/session passed 592 tests/subtests with no skips. Both vet commands passed. A freshly built Linux ARM64 worker exercised container integration. Fingerprints of 387 Hand and 266 Harness Go/module files were captured before and after and remained unchanged through the follow-up. Evidence: budget-full.json, raw race/vet logs and source fingerprints. This is development verification, not final coverage/platform/release qualification. Consolidated budget-usage.md around the actual current APIs and evidence, removing obsolete pending implementation claims while retaining remaining gates.

## Close manual-compaction and adapter run-budget bypasses

Regression tests found that manual compaction and the transitional Controller.Run adapter enforced session limits but omitted the selected run binding. Both now prepare the selected context, and direct HarnessBackend.Run also binds it before dispatch. Repreparation within Service remains idempotent. Cancellation cleanup forwards terminal errors instead of dropping them when its context is already cancelled, and releases bound timers after streams join. Tests prove zero summariser dispatch and unchanged history under an exhausted run, explicit resumption with a compaction-category charge, no Controller admission bypass, and actual deadline error retention/provider termination through both direct adapters. Scoped uncached race tests and vet passed; failing pre-fix tests retained. Evidence: run-compaction-integration.patch/json. Earlier broad and built-CLI/PTY evidence predates this delta; final qualification remains open.

## Resolve creation paths through existing ancestors

Nested new-file guidance testing exposed shared ValidatePathInWorkDir fallback to unresolved paths when the immediate parent did not exist. Replaced it with nearest-existing-ancestor resolution: legitimate missing descendants beneath symlinked workspace aliases are accepted, outside symlink ancestors are rejected, and dangling symlinks fail closed. Full tool/file race tests and vet passed. This removes the missing-parent validation defect; it is not a claim of descriptor-relative protection against concurrent ancestor replacement. Evidence: harness-creation-path.patch/json.

## Nested guidance before direct file mutations

Write/edit wrappers now discover applicable nested guidance before mutating. If guidance exists, the first call returns its contents/provenance and a path-bound digest without changing files; the agent reads it, adjusts its proposed mutation, and repeats with instruction_digest. Changed guidance invalidates old digests. The digest is not execution permission, and existing approval and isolation hooks remain required. Discovery resolves existing ancestors for new files/directories. Host registry and exact-path worker use the same wrappers. Tests verify no first-call mutation, stale-digest refusal, write/edit completion, worker parity, outside-path exclusion and cancellation. Initial new-parent test revealed and led to the shared Harness path-resolution fix. Focused race tests and vet passed; built worker/CLI journeys and broad post-change qualification remain pending. Evidence: mutation-guidance-integration.patch/json.

## Nested guidance on search results

M6.1 development search now returns applicable nested instructions with matched files, scoped by directory and deduplicated for multiple hits in the same directory. HAND/AGENTS precedence and collision diagnostics use the shared discovery code. Unmatched siblings are excluded. Whole guidance reports fit within the caller's existing max_bytes output allowance; omitted guidance is explicitly disclosed and instruction_guidance_complete is false. The separate search complete flag continues to describe match enumeration. Discovery checks cancellation and visits at most 128 matched directories.

The permanent TestSearchNestedGuidanceScopeAndBounds regression checks source precedence, collision disclosure, sibling exclusion, repeated-hit deduplication and bounded omission without partial instruction bodies. The final focused race suite passed 18 tests/subtests with zero failures/skips; vet passed. Earlier failed attempts are preserved: they exposed an incorrect collision-completeness flag and an undersized test output allowance, both corrected before the final run. Evidence: search-guidance-integration.json and its cumulative patch. Changes remain staged with unreleased Harness; built-worker journeys, broad post-change qualification and final acceptance remain pending.

## Built native worker guidance journey

The permanent check_guidance_worker.py runner now exercises read/skill resource discovery, guidance refresh, no first-call write/edit, stale-digest refusal, successful acknowledged mutations, nested search guidance, bounded omission and outside-workspace refusal despite a digest. A freshly built native macOS worker passed all 12 subprocess invocations with no provider calls. The fixture checked actual filesystem contents and absence of newly created parent directories on refusals. Raw request/response/stderr files and binary/runner digests are archived in guidance-worker-current.json. Archived Hand delta source hashes were checked against the build checkout. Container, permission-UI and exact released candidate qualification remain pending.

## RunTurn pin preservation audit and fix

M6.2 audit found Runtime.RunTurn omitted the durable objective/constraint pins already injected by Runtime.Run. Added the same dynamic prompt contribution, with pin decoding errors stopping provider dispatch. TestRunTurnPinsAfterCompactionRestartAndCorruption first reproduced the missing objective, then passed after the fix across three disk reopen/compaction cycles, asserting each exact pin appears once. It also checks corrupt versioned pin metadata cannot dispatch a provider request. All three focused pin tests passed under the race detector; runtime vet passed. Raw failing and passing runs are preserved in harness-runturn-pins.json; harness-runturn-pins.patch is the cumulative development Harness delta against the recorded base. This does not close structured preservation of unpinned decisions, unresolved work or verification references, or final released integration.

## Durable structured task state

M6.2 now has a versioned structured context annotation for objectives, decisions, unresolved work and evidence locators, bounded to 32 items/8 KiB. Idle replacements require the current annotation revision and persist through the session journal. Both Run and RunTurn reinject the state independently of summaries; manual and automatic compaction receive it; context inspection counts it. Corrupt/unsupported state blocks dispatch. Hand controller and RPC expose read/replace with strict parameters and durable replay. Eight Harness and two Hand focused race tests/subtests passed, with scoped vet passing. Cumulative artifacts: harness-context-state.patch/json and context-state-integration.patch/json. Semantics, RPC examples and remaining gaps are in structured-context.md. Automatic capture, terminal controls, built journeys and final integration/qualification remain pending; no requirement group is signed off by this evidence alone.

## Structured state terminal controls and PTY journey

Added /state inspection, selective set/evidence updates and removal, using controller-owned idle operations and a revision-checked replacement to preserve unrelated facts. Display sanitises recorded text and references and explicitly distinguishes evidence locators from verification. Focused terminal/RPC/pin tests passed (3 tests, no failures/skips), and app/TUI vet passed. A freshly built CLI passed the permanent eight-command real PTY journey: five actual journal updates, zero writes for inspection/invalid commands, final contents checked and clean exit. No prompts or paid calls. Raw terminal, report and session journal are archived in context-state-terminal.json. Cumulative source evidence: state-terminal-integration.patch/json. Automatic fact capture, broader qualification and released integration remain pending.

## Broader context regression validation

Ran uncached race suites for Hand app/TUI/RPC/agentio and Harness runtime/compaction. Initial Hand execution exposed sandbox denial for a local HTTP fixture and a real unconfigured-compaction regression introduced by run budget admission. Restored no-manager/no-summariser no-op handling before admission; configured compaction still checks budgets. Final Hand broad run: 639 passes plus one native-container skip; that test separately passed with the cached pinned container fixture. Harness: 383 passes, no skips/failures. Both scoped vet checks passed. All failed/skipped attempts are archived and not counted as passes. Evidence: context-broad.json and context-broad-integration.patch/json. This is scoped development validation, not the whole-suite, coverage or exact-candidate final gate. Automatic verification-reference capture was inspected but not changed during this run; it remains follow-up work.

## Automatic saved-verification context

M6.2 now retains the latest saved evidence reference per named verification profile in structured session state. Entries preserve profile/digest, snapshot IDs, exit code, assessment time and an explicit need to recheck current validity. Saved-evidence checks update the assessment, including stale after edits. Failed evidence saves leave context unchanged. Full state and conflicts with unrelated user facts refuse context updates visibly, preserving saved evidence and existing facts. The real owned-command regression now checks saved/stale context; a second regression covers capacity and collision preservation. Both passed with the race detector and app vet passed. Evidence: verification-context-integration.patch/json. General conversation fact capture, built CLI verification/context journey and broad post-change/final qualification remain pending.

## Built CLI automatic verification-context journey

Extended the permanent verification RPC runner and rebuilt Hand. The actual CLI passed across three client processes: saved pass/fail evidence, automatic context locators and snapshot scope, stale reassessment after edits, unchanged structured state after restart, replay preserving the stale assessment without rerunning the old verification, plus cancellation/disconnection joining the owned command processes. Provider calls: zero. Raw transcripts, records and session journals are archived in verification-context-rpc.json. This qualifies the development journey only; final released integration remains pending.

## M7 extension protocol framing started

Added the independent public extension/protocol Go package: versioned JSONL request/response envelopes, 256 KiB frames, required newlines, bounded IDs/depth, recursive duplicate-key rejection, strict envelope fields, UTF-8 validation, terminal reader failure and serialised writes with short-write errors. Permanent malformed-frame tests and a fuzz target passed under the race detector; vet passed. The 60-second fuzz campaign completed without a failure. Raw evidence and cumulative staged source are archived in extension-framing-integration.patch/json. The architecture record 0003-extension-protocol.md states the required host, policy, lifecycle, rendering and reload boundaries. M7.1 is now in progress; this transport alone does not complete any extension end-to-end acceptance scenario. Host integration, schemas, package trust, examples, transactional reload and qualification remain pending.

## Persistent extension transport ownership

M7.1 now has an admitted-transport Connection with generated request IDs, serialised calls, exact response matching, request timeouts, bounded stderr and joined termination. Unsupported extension callbacks fail closed until capability dispatch exists. Timeout/crash/protocol failure closes the peer; structured remote refusal leaves a healthy peer usable. Cancelling a queued call does not kill the active request. Real subprocess regression tests passed under the race detector: ten behavioural tests/subtests plus one helper-entry record in the raw log, no failures/skips. Scoped vet passed. Evidence: extension-connection-integration.patch/json. Product process admission/launcher, method schemas, callback capabilities, declarative rendering, transactional reload and examples remain pending. No end-to-end extension scenario is signed off by the connection foundation alone.

## Extension handshake and declarative schemas

Added strict typed payload decoding, known-capability validation, handshake identity/subset checks, bounded command/lifecycle registration, declarative presentation blocks and structured question/answer validation. Connection.Initialize validates before returning registrations and joins rejected peers. Real subprocess fixtures cover unapproved capability and identity expansion. Schema fixtures reject terminal controls, output/permission overrides, duplicate fields, unknown choices and ambiguous/cancelled answers. Protocol/connection race suite and vet passed; raw final log records 22 passing test/subtest entries, including helper/fuzz harness entries that are not independent product scenarios. Evidence: extension-schema-integration.patch/json. Product admission, namespace registration, callbacks, renderer/question UI, transactional reload and examples remain pending.

## Capability-gated extension callbacks

Added request-owned extension-to-host callback dispatch after validated handshake. Method-to-capability checks precede trusted handler invocation; denied methods never reach it. Duplicate/unsolicited/excessive callbacks terminate the peer, with a 32-callback per-request bound. Active handlers inherit request and connection cancellation, and cancellation joins handler/process cleanup. Idle-only handler configuration prevents replacement during work. Real subprocess fixtures verify allowed state calls, denied file access, duplicate IDs, the callback count limit, host policy refusal propagation and cancellation joining. Protocol/connection race suite and vet passed; final raw log has 29 passing test/subtest entries including helper/fuzz harness records. Evidence: extension-callback-integration.patch/json. Concrete Hand resource/question/state handlers, process admission, reload, UI and final qualification remain pending; declarations are not treated as sandboxing or resource permission.

## Extension-owned durable state

Implemented a state callback handler bound to trusted package identity and selected session, using versioned annotations and atomic revision checks. Package namespaces are isolated; peer input cannot select another owner. Payloads and encoded records are bounded, with 256 retained revisions and visible quota failure. Corrupt records are preserved and rejected. Tests cover concurrent writers, stale revisions, invalid JSON/duplicate keys, encoded-size expansion, cancellation, quota preservation, compaction/restart and real callback writes followed by peer/session restart. Protocol/extension race suite and vet passed; final raw log records 33 passing test/subtest entries including helper/fuzz harness records. Evidence: extension-state-integration.patch/json. Product activation/session binding, questions/resources, ordered transforms/policy hooks, reload and examples remain pending; no extension end-to-end scenario is signed off solely by this handler.

## Ordered transforms and restrictive policy hooks

M7.1 now has immutable host-owned hook ordering and mandatory flags. Context transforms receive only editable retrieval/tool-result contributions and can replace existing IDs within bounds; user/system/policy data remains unchanged. Invalid replacement batches are atomic, optional failures are diagnosed, and mandatory failures stop the action. Policy hooks run only after host allowance and cannot reverse host denial; explicit denials and mandatory crashes/missing decisions fail closed. Real subprocess tests verify deterministic tie ordering, caller-copy preservation, protected-ID rejection, atomic optional failure, zero dispatch on host denial and policy failure handling. Protocol/extensions race suite and vet passed. Evidence: extension-hooks-integration.patch/json. Runtime/tool-policy adapters, detailed operation schemas, activation, reload, UI and examples remain pending.

## Transactional extension registry reload

Implemented a bounded manager with trusted factory injection, specification snapshots, stable namespaces and idle-boundary reload. Healthy unchanged peers are reused; changed peers stage and validate before commit; staging failures close only staged peers and preserve the old active registry. Successful replacement/removal joins retired peers. Shutdown cancels active calls; reload refuses active calls. Added a guard against a faulty factory returning an active/staged peer and a final precommit health check. Real subprocess tests and the full protocol/extensions race suite passed; vet passed. Evidence: extension-reload-integration.patch/json. Transactionality does not roll back arbitrary startup effects. Product executable approval/launcher, method/hook adapters, terminal integration and examples remain pending.

## Typed manager commands, lifecycle and hooks

Added namespaced registered-command execution returning validated declarative blocks, subscribed lifecycle delivery with strict acknowledgement, and manager-owned context/policy pipelines that remain stable until work joins. Unknown commands are rejected before dispatch; presentation controls and outcome overrides are rejected. Tightened manager activation to require the reviewed capability contract exactly, preventing a peer from silently omitting a configured policy hook. Real subprocess tests plus the protocol/extensions race suite passed; vet passed. Evidence: extension-operations-integration.patch/json. Runtime/TUI/RPC adapters, executable trust/launcher, question/resource handlers and examples remain pending.

## Host-owned extension questions

Added a bounded session question broker, copied provenance/options snapshots, unique live response tokens and strict answer validation. Cancelled/expired tokens and replayed answers cannot affect later questions; close releases waiters. The callback handler binds the admitted extension identity and grants no resource authority. Broker tests and a real subprocess question/answer callback passed with the protocol/extensions race suite; vet passed. Evidence: extension-questions-integration.patch/json. UI/RPC delivery, interactive timeout policy, product launcher/runtime integration and examples remain pending; questions currently inherit the request deadline rather than waiting indefinitely.

### M7.1 reviewed host extension launcher

Implemented explicit launch review with executable/resource hashes, capability and configuration digest, explicit environment, private source snapshots and mandatory admission callback. Imported reviews are structurally revalidated independently of their digest, including resource paths, byte limits, arguments and environment. Host launch rechecks copied bytes before execution and owns process-group termination and snapshot removal. Connection close preserves transport cleanup errors. Real subprocess tests verify denied admission, admission-copy isolation, changed same-size resource rejection, joined cleanup and manufactured-review rejection. The protocol/extensions uncached race suite and vet passed after fixing a missing field and correcting fixture snapshot-directory permissions. Raw failed attempts are preserved in extension-launch-integration.json. No product acceptance scenario is signed off: application approval binding, container factory, UI/RPC/runtime wiring and Go/Python examples remain pending. Host execution is explicitly unrestricted; runtime libraries are not snapshotted.

### M7.1 Go and Python extension examples

Added two runnable task-note peers demonstrating a registered command, structured free-text question, revision-checked persistent state, declarative presentation and editable context transformation. The permanent integration journey compiles Go and launches both real peers through reviewed private snapshots, with a required Python 3 runtime. Each executes the command and transform, then restarts both process and disk session to verify state. Protected user context is preserved. Test-owned admission and structured answers are explicitly distinguished from product trust/UI evidence. The uncached protocol/extensions/examples race suite and vet passed; raw logs and the cumulative source patch are recorded in extension-examples-integration.json. The examples README documents build, runtime, protocol boundary and reproduction. Product activation, terminal questions, container execution and final qualification remain open.

### M7.1 application extension ownership and session binding (2026-09-09)

Added ExtensionHost in the application layer, binding admitted peers to the shared Controller operation owner, installed package identities and current session state. Commands and reload reserve cancellable running ownership. Pending questions and answers remain accessible during that reservation; other owner operations cannot overlap. Resource callbacks without concrete policy adapters fail closed. A permanent real-Go-peer integration test verifies reload/session-operation exclusion while waiting, answer replay refusal, copied identity configuration, state isolation after selecting another session and joined cancellation releasing both question and owner. The initial test exposed idle-state cancellation refusal; changed extension reservations to Running and reran the related application/extension/protocol race tests successfully. Vet passed. Evidence and cumulative source are in extension-app-integration.json/patch. Startup registration, CLI/TUI/RPC handlers, runtime hook attachment, container launch and final acceptance remain outstanding.

### M7.1 asynchronous RPC commands and questions (2026-09-09)

Added extension.command, extension.questions and extension.answer when a configured application extension host is present. Commands persist intent and terminal results using the durable control ledger, run outside the dispatcher mutex, and join on cancellation/disconnect. Answers are durable controls with live question-token validation. Real Go-peer tests exercise successful state writes, answer and command replay without duplicate effects, cancellation and disconnect without leaked pending questions. Related RPC/ledger/verification/context-state race tests and vet passed after correcting a test record-field name; the failed build is retained. Evidence: extension-rpc-integration.json/patch. Development API documentation: extension-rpc.md. Startup activation, terminal controls, runtime hook wiring, container launch and final qualification remain pending.

### M7.1 explicit reviewed RPC startup (2026-09-09)

Added --extension-config and --approve-extension-config for RPC startup. The selected regular nonsymlink file is bounded, strictly decoded and matched against a separately supplied exact-byte SHA-256 approval. Activation checks canonical workspace binding and rejects host execution when an isolation backend is selected. The configured ExtensionHost is attached before RPC serving and closed before runtime/session cleanup. Permanent tests reject missing/wrong approval, changed bytes, duplicate JSON, workspace mismatch and host fallback without creating snapshot resources. Scoped invocation and application race tests passed with authorised local HTTP listener access; vet passed. The initial sandbox listener failure is retained. Evidence: extension-startup-integration.json/patch. Review creation UX, a built-CLI extension journey, terminal support, runtime hooks, container launch and final qualification remain pending.

### M7.1 non-executing review generation (2026-09-09)

Added --review-extensions and --extension-review-output to fingerprint explicit launch input without provider construction or peer execution. The command creates a new private review file exclusively and prints its exact-byte approval digest; it does not approve activation. JSON launch inputs now have documented snake-case fields. Permanent tests verify review readback, no peer execution, existing-output preservation, cancellation and the CLI review path without provider configuration. Scoped race tests and vet passed. Launch file inspection/copy now uses nonblocking nonsymlink opens so changed file types cannot block on a FIFO open. Evidence: extension-review-integration.json/patch. Updated extension-rpc.md with Go and Python review instructions. Built-CLI activation journey, terminal controls, runtime hooks, container support and final qualification remain outstanding.

### M7.1 built Hand review/activation/RPC journey (2026-09-09)

The permanent scripts/check_extension_rpc.py runner passed against /private/tmp/hand-extension-cli-20260909 with both real Go and Python task-note peers. Each journey generated a review using the built CLI without execution, verified its exact digest, rejected a wrong approval without creating snapshots, activated explicitly reviewed test fixtures, completed a structured question and command, replayed the durable command after process restart without another question, cancelled a later command and verified empty snapshot directories after joined shutdown. Four real RPC clients ran; no provider calls were made. Binary, runtime, source and runner hashes plus raw RPC transcripts/review and startup diagnostics are archived in extension-cli-journey.json and development-evidence/extension-cli-20260909. Approval was test-owned and limited to these fixtures. This staged binary/local Harness evidence does not qualify the final candidate. TUI, runtime hooks, container execution and full acceptance remain pending.

### M7.1 asynchronous terminal commands and questions (2026-09-09)

Enabled explicitly approved extension startup in interactive mode and added /extension NAME COMMAND [arguments]. Commands run in joined cancellable session-operation workers. Generation-bound polling presents structured questions while work is active; Enter submits text or an option, Esc cancels the question, and Ctrl+C reaches the existing operation cancellation path. Question answers bypass the ordinary busy Enter guard without allowing model/session commands to overlap. Completion clears question state and stale poll messages stop. Declarative text/code/list content is sanitised into transcript notices; richer rendering remains pending. A real Go-peer TUI model test confirms question entry, persisted state and joined ownership release; answer mode and related session cancellation tests plus vet passed. Evidence: extension-tui-integration.json/patch. Native PTY qualification, reload integration, runtime hooks, container execution and final acceptance remain outstanding.

### M7.1 native terminal question and cancellation journey (2026-09-09)

Added scripts/check_extension_terminal.py and ran the built Hand binary in a real 140x45 macOS PTY with the Go task-note peer. Seven steps passed: invoke command, submit free text, invoke again and cancel the question with Esc, invoke again and cancel the command with Ctrl+C, then inspect /state to confirm the core remained usable. The session journal contains exactly one extension state revision with the submitted note; both cancellation paths wrote no additional state. Hand exited zero and the private snapshot directory was empty. Raw terminal bytes, reviewed fixture configuration and hashes are archived with extension-terminal-journey.json. No model prompts or paid calls occurred. This is native development evidence, not final candidate or Linux qualification. Rich rendering, reload integration, runtime hooks, container launch and remaining acceptance remain pending.

### M7.1 reviewed transactional reconfiguration (2026-09-09)

Reload can now stage a replacement admitted factory and commit it atomically with the peer registry. The previous factory and healthy peers survive failed staging; successful reload retains unchanged healthy peers. Application ReloadFile revalidates exact-file approval and workspace, binds callbacks to copied package identities and rejects per-name identity rebinding during the session. An explicitly reviewed empty set removes all peers. Added /reload REVIEW_FILE APPROVED_SHA256 and asynchronous durable extension.reload RPC; the RPC result preserves the committed report even when retirement cleanup reports an error. Permanent real-peer tests cover factory commit/rollback, capability-handshake failure preserving the old peer, reuse, identity rejection and remove-all cleanup. Related application/RPC/TUI race tests and vet passed. Evidence: extension-reconfigure-integration.json/patch. Built-client reload journeys, rich rendering, runtime hooks, container extension execution and full acceptance remain pending.

### M7.1 built RPC and native terminal reload journeys (2026-09-09)

Extended the permanent RPC runner and passed both Go and Python peers through unchanged-peer reuse, failed reviewed capability staging with unchanged working snapshot, cancellation, reload recovery, another usable command, explicit remove-all and rejection of commands targeting the removed peer. Existing review/approval, question, durable replay and cleanup checks remain active. Extended the native PTY runner to reload after Ctrl+C and invoke the recovered peer; all ten steps passed with exactly one persisted note revision and no surviving snapshots. Both used the same newly built Hand binary, SHA-256 2c0ae85de20bb3be76487f0157e3682b9eaddd81e68fc0b693ffebc6b62117f0. No provider prompts or paid calls occurred. Raw RPC/terminal evidence and fixture review files are archived in extension-reload-journeys.json. Final candidate, runtime hooks, richer presentation, container execution and complete acceptance remain outstanding.

### M7.1 framing expansion regression and fresh fuzz run (2026-09-09)

Corrected the frame-reader fuzz oracle: accepted input can exceed the output limit after JSON HTML escaping, so the writer is required to reject oversized re-encoding without emitting bytes, while successful writes must remain readable. Added a permanent explicit expansion regression. Protocol limits and product behaviour were not relaxed. The full scoped protocol/extensions race suite and vet passed. A fresh 60-second fuzz run completed 28,800,354 executions and passed (61.468 seconds total tool-reported test duration). Raw fuzz log and test evidence are archived in extension-framing-boundary-integration.json. This qualifies the development parser snapshot only; final candidate fuzzing and the rest of the acceptance contract remain required.

### M7.1 reusable Harness tool-context projection (2026-09-09)

Added LifecycleHooks.TransformToolContext and routed Run/RunTurn provider calls and retries through a request-copy helper. The hook receives only tool-result text and host-owned message indices; replacements must preserve indices/count and remain valid UTF-8 within 256 KiB per entry. It cannot alter tool-call identity, error flags, system/ordinary user/assistant messages or the original message slice/session journal. Validation failures and cancellation stop before provider admission. Builder hook overrides include the new hook. Tests cover source-copy preservation, protected-index rejection, missing entries, oversize output, error and cancellation; related RunTurn/retry/builder race tests and vet passed. Fixtures use Harness's actual user-role-plus-tool-call-ID representation of tool results. Evidence: harness-tool-context.json/patch. This is an unreleased reusable API; Hand extension attachment and integrated runtime transform journeys are still pending, as is full acceptance.

### M7.1 runtime extension context attachment (2026-09-09)

Reviewed extension startup now attaches the manager's ordered context pipeline to Harness.TransformToolContext. Tool text is copied and batched at 64 items/64 KiB, with explicit rejection above the per-contribution 16 KiB extension limit. No active transform leaves contributions untouched without imposing extension limits. Optional failures are diagnosed; mandatory pipeline failures propagate before provider admission. Existing host context hooks are composed first. A real Go task-note peer transforms the actual provider request in both Run and RunTurn while original journal entries and ordinary user content remain unchanged. The same peer passes 130-item batching with stable indices and copied inputs. Scoped application/extensions race tests and vet passed. Evidence: extension-context-integration.json/patch, requiring harness-tool-context.patch. Lifecycle/policy attachment, richer presentation, container extension execution and full candidate qualification remain pending.

### M7.1 runtime policy veto attachment (2026-09-09)

Attached extension policy checks through a Runtime.Permission wrapper that first preserves the existing host decision and tool-definition filtering. Host denial does not contact the extension. Allowed actions send tool.execute, the tool name and bounded strict original JSON arguments to the negotiated policy pipeline. Explicit veto or mandatory failure returns a normal tool denial. Added a small real Go policy example with documented exact-input-path matching. The integrated RunTurn test proves a denied tool was never executed and has a paired recorded denial result, a distinct path is allowed, a closed mandatory host denies and the original host denial takes precedence. Scoped policy/context/hook race tests and vet passed after completing the test executor interface; failed build retained. Evidence: extension-policy-integration.json/patch. Lifecycle attachment, broad streaming/parallel policy journeys, richer presentation, container support and final acceptance remain outstanding.

### M7.1 serial and streaming policy admission regression (2026-09-09)

Expanded the real policy-peer test to RunTurn, serial Run and streaming Run. The streaming fixture is concurrency-safe and keeps the provider stream open until the permission wrapper has completed, proving the streaming kickoff path reaches extension admission before provider completion. Each path prevents denied tool execution, records a denial, allows an unrelated exact path, fails closed after policy-host shutdown and preserves the host-denial reason. One scoped race run and 20 additional uncached race repetitions passed; the repeated log contains 80 parent/subtest pass records (three execution paths plus parent per repetition), not 80 distinct acceptance scenarios. Vet passed. Evidence: extension-policy-paths-integration.json/patch. Lifecycle wiring, parallel-batch qualification, richer presentation, container support and full final-candidate acceptance remain outstanding.

### M7.1 reusable run lifecycle admission and observation (2026-09-09)

Added Harness OnRunStart/OnRunFinish hooks to Run and RunTurn, including builder override handling. Start runs after budget setup but before user-history append or provider work and can abort admission. Finish observes the established reason once per accepted invocation and receives a cancellation-independent context capped at two seconds; implementations must respect it. Both callbacks contain panics; finish observer panic cannot replace the run outcome. Tests cover success/start denial, exactly-one start/finish pair, no user-history mutation on start denial and finish panic containment in both execution APIs. Related lifecycle/RunTurn race tests and vet passed. Evidence: harness-run-lifecycle.json/patch. Hand subscription attachment and session/compaction events remain pending; this is unreleased development API evidence, not full candidate acceptance.

### M7.1 run lifecycle subscription attachment (2026-09-09)

Reviewed startup now attaches subscribed run.start/run.finish notifications to the reusable Harness lifecycle hooks. Start delivery composes with existing host admission and propagates mandatory failure before provider work. Finish delivery is diagnostic only and preserves the established outcome, including when an existing finish observer panics. Notifications include the selected session ID and finish reason. Real subprocess fixtures verify ordered exactly-one notification pairs in both Run and RunTurn, no provider call after mandatory start refusal, and successful outcome preservation after mandatory finish observer error. Related context/policy race tests and vet passed. Evidence: extension-lifecycle-integration.json/patch, requiring harness-run-lifecycle.patch. Session/compaction notifications, extension question delivery during model runs, richer presentation, container support and final acceptance remain outstanding.

### M7.1 extension questions during model runs (2026-09-09)

Extended TUI question polling to active model and queued runs, using an independent monotonically increasing generation rather than the session-operation counter. Finishing a goal invalidates old polls. Enter/Esc can answer the live extension question while the run remains owned; Ctrl+C retains normal cancellation. Existing typed drafts are saved when a question appears and restored after answer, expiry or completion. A real run-start subprocess observer asks a structured question during RunTurn; the model test proves delivery during running state, empty answer input, submitted answer, draft restoration and stale-poll rejection. Related application/queue/session ownership TUI race tests and vet passed. Evidence: extension-run-question-integration.json/patch. Native model-run question qualification, session/compaction lifecycle events, richer presentation, container support and full acceptance remain outstanding.

### M7.1 activation ownership and repeated-hook prevention (2026-09-09)

Activation now holds the shared cancellable operation reservation through reviewed peer startup and runtime hook attachment, removing the prior gap after reload released ownership. Once successfully activated, a controller rejects subsequent activation, including after host shutdown; explicit reconfiguration uses the existing host's transactional reload. Failed admission leaves activation retryable. The permanent regression checks busy refusal, no snapshot creation on refusal, successful retry after boundary rejection, no permission replacement or resource creation on repeated activation, and owner release after refusal. The focused application race suite passed with nine parent/subtest pass records; 20 additional uncached ownership regression repetitions and vet passed with no failures or skips. These are development checks, not distinct acceptance scenarios or final candidate qualification. Evidence: extension-activation-integration.json/patch, requiring harness-run-lifecycle.patch. Session/compaction lifecycle events, richer presentation, container execution and full acceptance remain outstanding.

### M7.1 typed extension presentation and resize (2026-09-09)

Extension command results now enter the transcript as immutable typed text, code and list-item sources, with a host-owned extension attribution notice. Code uses a language label and neutral border; list continuation lines use hanging indentation. Plain text and code remain literal, including Markdown link syntax and code fences; extensions cannot select transcript approval states or terminal styles. All content passes strict presentation validation before conversion and defensive terminal sanitisation on rendering. Source-backed resize preserves original content and reflows at current width. Regression tests cover widths 1/2/5/6/12/40, preserved indentation/fences, list source copying, resize, clipboard/screen escape rejection, bidi rejection and unknown approval block refusal. The real Go-peer command/question test now asserts typed presentation delivery after joined completion. The final focused TUI race suite passed with nine test records, no failures or skips; vet passed. Evidence: extension-presentation-integration.json/patch. Native rich presentation qualification, session/compaction lifecycle events, container execution, package management and full acceptance remain outstanding.

### Broad validation after extension integration (2026-09-09)

Ran uncached whole-Hand race tests against the staged extension-presentation source. The sandbox run recorded 901 passing test records but stopped cmd/hand, sdk and internal/app at prohibited local HTTP binds; those complete packages passed with authorised local-listener access (261 passing records, three unconfigured native fixture skips). Built a fresh Linux/arm64 worker and ran all four native fixture skips, including toolproxy, with cached local Docker images. The first native invocation used repository-digest syntax rather than the backend-required local image-ID syntax and was rejected before execution; corrected sha256 image IDs passed all four tests. Replacing prematurely stopped package records and native skips yields 1,074 distinct passing Hand package/test-name records, zero remaining test failures/skips. Six package-level no-test-file events are not counted as passing tests. Parent/subtests and subprocess helpers remain records, not distinct acceptance scenarios. Harness runtime/compaction race tests passed with 395 records; whole-Hand and scoped Harness vet passed. All raw attempts, source hashes captured after testing, worker hash and explicit reconciliation are in extension-broad.json. The staged source matches extension-presentation-integration.json hashes. This is broader development validation, not final candidate coverage/platform/performance/live qualification; the released Harness and clean integration gates remain open.

### M7.1 human-interaction deadlines and joined cancellation (2026-09-09)

Extension commands and run.start delivery now receive a five-minute operation ceiling at both manager and connection layers, shared across callbacks rather than renewed by another question. Handshake/reload, policy checks, transforms and other lifecycle notifications retain the 30-second protocol ceiling. A shorter parent deadline always wins, including the runtime's two-second finish observer deadline. Real subprocess callback tests observe the propagated deadline for commands, run.start, run.finish, policy and a shorter parent; cancellation then proves callback join and registry ownership release. The focused race suite passed 19 test records. Twenty further uncached repetitions passed 120 parent/subtest records (five cases plus parent per repetition), with no failures/skips; vet passed. These deadline observations do not claim an elapsed five-minute expiry test or native interactive timeout journey. Evidence: extension-timeout-integration.json/patch. Remaining lifecycle delivery, container support, package management and complete candidate qualification remain open.

### M7.2 package manifest and directory integrity foundation (2026-09-09)

Added internal/packages with a strict version-1 manifest covering package/version compatibility, complete content inventory, skill/prompt/extension/source/asset roles, interpreter requirements, extension entrypoints and capabilities. Canonical SHA-256 identity sorts inventories and capability sets while preserving argument order and caller data. Validation bounds both input and encoded manifests at 256 KiB, rejects duplicate/unknown JSON, traversal, case/path collisions, invalid versions, oversized content and undeclared runtime/entrypoint references. VerifyDirectory uses an os.Root boundary and nonblocking regular-file opens, refuses symlinks/unlisted/missing files, hashes bounded file reads with cancellation and verifies executable modes. Inspection explicitly does not freeze mutable source or grant execution trust. Added a content-pinned manifest for the existing real Python task-note example and a permanent integrity test. The final package race run passed 14 parent/subtest/seed records, with no failures/skips; vet passed. A fresh 60-second manifest fuzz run passed 26,357,485 executions (61.433 seconds reported duration), before the example-only test addition and with unchanged production parser code. Evidence: package-manifest-integration.json/patch. Contract: package-manifest.md. M7.2 is in progress; local/Git/archive installation, runtime checks, lockfiles, capability review, update/remove/rollback, CLI integration and full acceptance remain open.

### M7.2 pinned private local staging (2026-09-09)

Added StageDirectory and owned Snapshot cleanup. Staging requires an explicit expected digest and private nonsymlink parent outside source, verifies input, revalidates every copied file's bytes/size/executable mode through confined roots, writes read-only content and manifest, verifies the resulting snapshot and syncs files/directories. Cancellation or failure removes owned partial output and reports cleanup errors. Tests verify snapshot independence after source edits, idempotent cleanup, wrong/missing pin refusal, unsafe parent refusal, cancellation injected after a partial file appears, and changed content/size/mode/symlink rejection at copy time. Initial tests assumed t.TempDir permissions were 0700; fixtures now explicitly set the required private mode without weakening product checks. Final race tests passed 27 records; 20 further cancellation/source-change repetitions passed 120 parent/subtest records, no failures/skips. Vet passed. Evidence: package-stage-integration.json/patch. Staging does not commit an installation or grant execution trust; store/lock transactions, runtime checks, Git/archive import, updates/removal/rollback, CLI and full acceptance remain open. No power-loss durability qualification is claimed.

### M7.2 compatibility and reviewed runtime probing (2026-09-09)

Added semantic-version precedence and Hand minimum-version validation, including prerelease ordering, ignored build metadata and overflow-free numeric comparison. Added non-executing runtime discovery/fingerprinting with actionable missing-command diagnostics, plus separately approved Python/Node/Ruby version-probe adapters. Probes copy/revalidate the approved executable privately, use fixed --version arguments/minimal environment, enforce five-second and bounded-output limits, compare the result with the declared minimum and join process-group cleanup. Runtime libraries are explicitly outside the snapshot boundary. A compiled subprocess fixture proves review/wrong-approval paths do not execute, approved version retrieval, too-old rejection, changed-review approval refusal, changed-executable refusal and cancellation after the process signals readiness. Final package race tests and vet passed; initial incorrect Capture API build attempt retained. Evidence: package-runtime-integration.json/patch. Native interpreter qualification, installer/activation integration, store/lock transactions, imports, update/remove/rollback and final acceptance remain pending.

### M7.2 reviewed local store, updates and rollback (2026-09-09)

Added a private single-writer package store with bounded versioned lock metadata, retained content objects and atomic generation updates. PrepareInstall/PrepareRollback/PrepareRemove return concrete reviews; Apply requires separate full-review digest approval binding store identity, action, prior generation/current pin, source and manifest. Capability-changing updates require new approval. Installation revalidates staged bytes and compatibility; rollback revalidates retained content; removal changes the installed inventory without executing packages or touching source. Files/directories sync before lockfile replacement; Committed remains true if a post-rename error occurs. Unreferenced content from an unsuccessful later lock commit stays inactive and retained for future collection. Tests cover install/update/reopen/rollback/remove, writer exclusion, wrong/stale/cross-store approvals, source changes, corrupt objects/locks and cancellation immediately before rename preserving exact prior bytes and removing temporary lock output. Final package race run passed 34 records; 20 further store repetitions passed 80 records with no failures/skips; vet passed. Initial unused-import build failure retained. Evidence: package-store-integration.json/patch. Git/archive import, CLI and runtime activation wiring, orphan collection, process-crash/power-loss qualification and full acceptance remain open.

### M7.2 pinned archive import (2026-09-09)

Added explicit local ZIP/tar/tar.gz/tgz import with independent archive-byte and package-manifest pins. Import copies/revalidates archive bytes privately before extraction, bounds index/entry/content sizes, rejects links/special files/traversal/duplicates, verifies gzip checksums and trailing tar padding, then uses the existing directory verifier and private staging API. Supports ordinary ./ tar roots without accepting absolute roots. ZIP64/multipart layouts are explicitly refused; central-directory allocation is bounded before archive/zip indexing. Manifest validation now also rejects case-colliding parent directory spellings for portable inventories. Tests import all four formats and apply their snapshots through reviewed store installation; refusal cases check cleanup for traversal, absolute files/directories, links, unlisted/duplicate content, wrong pins, corrupt gzip, hidden trailing data and oversized declared files. Final package race tests and vet passed with no failures/skips. Evidence: package-archive-integration.json/patch. Original-source provenance wiring, Git import, CLI/activation integration, crash/platform qualification and full acceptance remain open.

### M7.2 pinned local Git commit import (2026-09-09)

Added ImportGit for explicitly selected local checkouts/bare repositories using full SHA-1 commit and package SHA-256 pins. Fixed bounded read-only host Git commands check object type and committed modes, reject symbolic refs/symlinks/submodules, disable replace refs and avoid checkout/hooks. Exported tar passes ordinary archive/package verification and staging. Real Git tests modify the working tree and add a checkout-hook sentinel, then prove pinned committed bytes are imported without running the hook or changing working files; imported content installs through the reviewed store. Ref/unsupported-mode/wrong-pin failures clean temporary output. The first integration test exposed Git's global tar commit comment; added narrowly bounded support while rejecting global path overrides. Final package race tests passed 60 records, no failures/skips; vet passed. Offline Go Git client dependency attempts and the initial tar-header rejection are retained. No go.mod dependency changes were needed (only the existing local Harness replacement differs from primary). Evidence: package-git-integration.json/patch. Remote Git transport, original-source provenance, CLI/runtime activation wiring and full acceptance remain open.

### M7.2 durable import provenance and legacy locks (2026-09-09)

Snapshots now retain importer-provided origins: local directory, archive location/SHA-256 or local Git repository/full commit. PrepareSnapshot holds snapshot lifetime ownership during verification, copies origin into the complete change review and binds it into approval. Retained revisions preserve that metadata after snapshot cleanup and through rollback. Existing version-1 lockfiles without origin remain readable and keep provenance unknown; no origin is invented from a staging path. Plain local installation still records its selected source. Real archive/Git installation tests now use PrepareSnapshot and verify persisted origin after temporary files are removed. Additional regressions reject changed-origin approval reuse, closed snapshot review and invalid provenance, and prove legacy-lock rollback preserves absent metadata. Package race tests and vet passed with no failures/skips. Evidence: package-origin-integration.json/patch. Remote Git transport, CLI/activation wiring, crash/platform qualification and full acceptance remain open.

### M7.2 real writer-death commit-boundary regression (2026-09-09)

Added a permanent real-subprocess store crash fixture. The parent first commits an installation, then launches a separately locked writer that removes it. One case pauses after the replacement lock has been synced but before rename; the other signals readiness after committed removal. The parent confirms live cross-process writer exclusion, kills and joins the child, reopens the store and checks generation/current package state and retained object hashes. Before rename, the exact previous lock bytes remain authoritative; the leftover temporary lock is ignored, and a subsequent explicit removal succeeds. After commit, the removal survives writer death. Focused race tests passed; 20 further repetitions passed 60 parent/subtest records covering 40 killed writers; vet passed. Evidence: package-crash-integration.json/patch. This is native development process-death evidence, not power-loss or native Linux qualification. Stale temporary-lock cleanup, remote Git transport, CLI/activation integration and full acceptance remain open.

### M7.2 bounded stale lockfile recovery (2026-09-09)

Store opening now recovers abandoned generated lock-<base32>.tmp files after acquiring the exclusive writer lease and validating committed lock metadata. A bounded root scan precedes removal; only private 0600 regular files within the lockfile byte limit are removed, and the directory is synced afterward. Symlinks, directories, different permissions and unrelated names remain untouched. Corrupt stores are refused without cleaning temporary evidence. RecoveryReport exposes removed and preserved-unsafe counts. Real writer-death tests now verify the pre-rename leftover is removed on reopening while exact prior committed bytes and retained content survive. Permanent tests cover preservation, symlink-target safety, idempotence and corrupt-store refusal. Full package race tests passed; 20 further recovery/crash repetitions passed with no failures/skips; vet passed. Evidence: package-recovery-integration.json/patch. Staging/object garbage collection, power-loss/native Linux qualification, remote Git transport, CLI/activation integration and full acceptance remain open.

### M7.2 local package CLI and built-client journey (2026-09-09)

Added early `hand packages` dispatch with inspect/list, local review-install (including updates), review-rollback, review-remove and separately approved apply. Strict per-command flags and bounded exclusive private review files are checked before provider setup. Apply validates approval before opening the selected store and emits the committed result even alongside an error. Store inspection/removal is now available for development builds, while installation/rollback still fail unknown-version compatibility checks. Permanent handler tests exercise full install/update/rollback/remove, stale/wrong approval, review-file preservation, irrelevant flags, development inspection and dispatch. Scoped package/CLI race tests and vet passed. Built /private/tmp/hand-package-cli-20260909 with explicit development test version 1.0.0-dev.package-cli; the permanent scripts/check_package_cli.py runner passed 17 real CLI commands, ending at generation 4 with no installed package or staging residue. It checked wrong/stale approval and changed-capability reapproval, with zero provider calls or extension executions. Binary SHA-256: 11f482d46f190c93ce58390aaf1865a2358c69d75d76c8c6779a7f3d13fd9a7c. Raw commands/reviews/lockfiles and hashes: package-cli-journey.json; cumulative source/tests: package-cli-integration.json/patch. Imported-source CLI, runtime/activation wiring, remote Git and full qualification remain pending.

### M7.2 archive/Git CLI reviews with revalidated application (2026-09-09)

Added review-archive and review-git with archive-byte/full-commit pins in addition to package identity. PrepareSnapshot now names original imported sources in reviews; Apply reconstructs and verifies the original archive/Git source before committing, so review-generation snapshots can be removed immediately. Rollback still uses retained objects. Handler tests cover unavailable original sources causing no commit, successful retry after restoration, durable provenance and empty staging. Scoped package/CLI race tests and vet passed. The permanent package CLI runner was extended and passed 23 commands against a new development binary, including both archive and local Git review/application, with no provider calls or extension execution. Binary SHA-256: c3f53f848371c9b4ffae3a39eb533526cd29f0a80983ccb95dca6492d89d88e9. Raw command/review/store evidence and a Git bundle preserving the fixture commit are archived in package-import-cli-journey.json. Source/tests: package-import-cli-integration.json/patch. Remote Git transport, runtime/activation wiring and full qualification remain open.

### M7.2 verified installed selection for activation preparation (2026-09-09)

Added ResolveSelected for bounded explicit installed-package selection under one store generation. It verifies current object directory type, complete content and digest, package/version identity and Hand compatibility; duplicate/missing packages, cancellation and corruption produce no partial results. Fresh manifests do not share mutable caller state. Added `packages show --name` to expose this verified selection through the CLI. Regression cases cover modified content, substituted object-directory symlinks, inconsistent lock version metadata, development incompatibility, cancellation, duplicates, missing selections and caller mutation. The CLI lifecycle test confirms the shown pin/generation after installation. Focused race tests and vet passed; evidence: package-resolve-integration.json/patch. No new built-client qualification is claimed for show. Package-to-launch conversion, runtime/activation wiring, remote Git and final acceptance remain open.

### 2026-09-09 — Installed package launch review bridge

Added development `ReviewPackageExtensions`: explicitly selected installed packages are revalidated, runtime reviews require matching separate approvals, and package content is converted to the existing startup review with stable package/extension identities. Policy extensions are mandatory. Review does not activate extensions. Executable resources now remain in the reviewed asset inventory; only an independently snapshotted native entrypoint without an asset argument is omitted. Permanent regression coverage checks executable helper integrity, identity, no execution or activation, and invalid selections/approvals. Raw race results and cumulative source fingerprints are in `package-launch-integration.json` and its linked evidence. Installed-package activation journeys and CLI wiring remain unfinished; M7.2 remains in progress.

### 2026-09-09 — Installed Go and Python activation journeys

Permanent `TestInstalledPackageExtensionActivationJourney` builds the shipped Go example or packages the Python example as an executable interpreted entrypoint, installs the content pin, deletes the source, converts the retained package to a launch review and approves the exact startup file bytes. Each language starts two fresh processes with reopened exclusive sessions, answers real protocol questions, verifies durable state revisions and checks snapshot cleanup after joined shutdown. Python runtime discovery/probing is explicitly approved within the test fixture. Both language cases passed under race detection; app vet passed. The initial test compilation failure (revision int/uint64 mismatch) is preserved alongside the corrected results. See `package-activation-integration.json` and its raw evidence. These are development journeys, not final candidate or container qualification; CLI wiring remains pending.

### 2026-09-09 — Installed package CLI launch reviews

Added `packages review-runtimes` and `packages review-extensions`, preserving separate runtime-probe and extension-startup approval boundaries. Runtime review outputs do not grant approval. The launch review emits the exact startup byte digest consumed by the existing startup/reload path. Package review writing now removes incomplete outputs on write/sync/close failure. Permanent CLI tests cover unapproved refusal, real approved Python probe, startup digest/identity, no activation and exclusive-file preservation. A strict-reader array/object format defect was found and fixed; failed and corrected raw results are retained in `package-launch-cli-integration.json`. Scoped race tests and cmd vet passed. A fresh built-client journey and final qualification remain outstanding.

### 2026-09-09 — Fresh built package CLI review journey

Extended the permanent package runner and built Hand with version `1.0.0-dev.package-launch-cli`. All 28 commands passed, covering installed manifest resolution, unapproved runtime refusal, explicit fixture-owned Python probe approval, exact startup-file SHA-256, private file permissions and existing-file preservation, alongside the existing install/update/rollback/remove and archive/Git import paths. No provider calls or extension startup occurred in this runner. Original local Git history is retained as a bundle. Raw commands, review documents, archive, bundle and hashed report are archived under `development-evidence/package-launch-cli-journey-20260909`; `package-launch-cli-journey.json` binds the binary and runner hashes to the source aggregate. Source fingerprints were rechecked after the run. Built-client activation and final candidate qualification remain outstanding.

### 2026-09-09 — Built-client installed Python package activation

Added permanent `scripts/check_package_activation_rpc.py`. Using the freshly built Hand binary, it installs the shipped Python package with its content pin, deletes the original source, explicitly approves the fixture interpreter probe, generates an exact-byte startup review, and activates it through RPC. The real peer saves a note through question/answer, the second Hand process replays the durable completed command without asking again, cancellation joins an active question, and both disconnects leave no launch snapshots. The journey passed with no provider requests. `package-activation-rpc.json` links binary/runner/dependency hashes, full RPC transcripts, review files and fixture session records. This establishes the host Python built-client path; final platform, container and candidate qualification remain pending.

### 2026-09-09 — Verified installed text resources

Added `ReadTextResource` and `packages read --name PACKAGE --path RESOURCE [--store STORE]`. Explicitly selected skill/prompt files carry package digest, store generation and text digest; package integrity is revalidated and returned bytes are rehashed. Invalid UTF-8, over-64-KiB content, unlisted paths, other file roles, corruption and cancellation fail without exposing partial text. Permanent resource and CLI tests passed under race detection. Evidence is recorded in `package-resources-integration.json`. Skill discovery and prompt application integration remain pending; loading text does not grant execution authority.

### 2026-09-09 — Application-owned package skill selection

Added `SelectPackageSkills` for explicit named installed skill selections, bounded to 64 skills/256 KiB and captured as immutable verified text. Authored workspace/personal skills retain precedence; package digests remain in context provenance. Failed or busy selections preserve the existing provider. Empty selection removes package skills; installation removal does not silently mutate an active text snapshot, and reselection requires the package still be installed. The permanent integration test exposed original-provider closure capture in Harness; the cumulative `harness-skill-provider.patch` makes lookup and index refresh consult the current provider. Corrected application race tests, focused Harness skill race tests and Harness runtime vet passed. Initial failure is retained. See `package-skills-integration.json` and `harness-skill-provider.json`. CLI/RPC selection wiring, persistence and prompt application remain pending.

### 2026-09-09 — Coherent package text batches and actual tool lookup

`ReadTextResources` resolves all selected package identities against a single inventory generation, then reads content-addressed resources in selection order with per-file and aggregate quotas. Invalid resources return no partial text. Package skill selection now uses this batch API. Permanent tests cover consistent generation, duplicate requested content and all-or-nothing failure. A Harness regression invokes the actual registered `load_skill` tool after provider replacement and removal, checking the refreshed index and tool output together. Its initial expected output omitted disk-provider frontmatter; that test expectation was corrected and the failed raw result retained. Scoped Hand and Harness race tests passed. Cumulative manifests are `package-skill-selection-integration.json` and `harness-skill-selection.json`; final qualification remains pending.

### 2026-09-09 — Digest-pinned package skill startup

Added `--package-skills` with strict versioned JSON, an absolute store and explicit package/path/name/digest selections. Runtime construction applies it before interactive/RPC/one-shot execution. Application selection now requires each verified package digest to match its pin, preserving the old provider on mismatch. CLI integration installs a real skill resource, reads the startup file and invokes the actual registered `load_skill` tool; missing pins and wrong-pin replacement are rejected. Scoped race tests passed. `package-skills-startup-integration.json` records the cumulative source and evidence. Fresh built-client selection, RPC reconfiguration and prompt application remain pending.

### 2026-09-09 — Pinned package prompt input

Added `--package-prompt` for one-shot execution and a context-carried prompt snapshot in the shared input parser. Exact installed digest and prompt role are required. Package instructions retain provenance at user level, without recursive attachment, slash-command or shell expansion; the user's own attachments still use the existing checked parser. Tests verify preserved literal text, metadata and wrong-pin rejection. Scoped race tests passed; evidence is in `package-prompt-integration.json`. Session cleanup is registered before startup skill selection so rejected configuration releases the session. Interactive/RPC prompt selection and fresh built-client qualification remain pending.

### 2026-09-09 — Package integration regression qualification

Ran uncached race suites for Hand packages, agentio, app and cmd/hand, plus the complete Harness runtime suite. Initial Hand execution recorded 348 passing records before two local HTTP fixture sandbox failures; the affected app/cmd suites were rerun with loopback access (257 pass, one unconfigured native-container skip). The native background-container input/output/isolation/shutdown test then passed against the existing local Docker image. Reconciliation by package/test identity yields 855 distinct passing records across the stated Hand/Harness scope, with no remaining failures/skips. Both scoped vet commands passed. Raw failures/skips remain archived. Current source fingerprints match the package-prompt and Harness skill-selection aggregates. See `package-broad.json`; this is development regression evidence, not whole-repository, scenario-count or final-candidate completion.

### 2026-09-09 — Built package skill and prompt provider journey

Permanent `scripts/check_package_text_cli.py` installs pinned skill/prompt content, removes the source, starts the fresh Hand binary with both startup selections and exercises the actual provider request and `load_skill` response. The local provider fixture confirms prompt text at user level, skill index provenance at system level, tool-loaded body, literal attachment/shell syntax and wrong-pin rejection before provider access. Two local requests, zero paid calls. Initial fixture minimum-version mismatch and default scoped tool denial were retained; the successful isolated fixture explicitly uses `--yes` and requests only `load_skill`. Binary and both source aggregate fingerprints are recorded in `package-text-journey.json`, alongside all three run artifacts. This is development behaviour evidence, not model-quality or final acceptance qualification.

### 2026-09-09 — Verification hook example

Added the Go `verification-hook` example with a fixed `verify-whitespace` command and `run.finish` subscription. It checks staged and unstaged tracked Git whitespace within one shared second, disables external diff/text conversion, bounds output and uses joined process execution. Its result explicitly excludes tests/untracked files and cannot override the existing run outcome. Real Git fixtures cover clean/staged/unstaged cases, cancellation and invalid repositories; protocol declarations and unsupported inputs are tested. Race tests passed after fixing test syntax and validation-call errors, both preserved in the raw evidence. See `verification-example-integration.json`; installed packaging and host lifecycle journey remain pending.

### 2026-09-09 — Installed verification hook host journey

The permanent application journey now builds the verification hook, constructs and installs a content-pinned manifest, deletes the original executable source directory and activates the retained installed package through launch review conversion. Real Git changes produce passing and failing command presentations; clean finish delivery has no diagnostics, failed verification produces an attributed diagnostic, and the actual attached runtime finish hook returns normally. Joined shutdown removes launch snapshots. Both the initial host test and expanded installed-package journey passed under race detection. `verification-host-integration.json` contains the cumulative source and raw evidence. Final candidate/platform qualification remains pending.

### 2026-09-09 — Explicit tool result viewer example

Added a Go tool-viewer example with typed text/list/JSON-code presentation. It accepts a bounded explicitly supplied tool/output/error record and labels it as supplied, unverified execution data. It neither invokes tools nor changes run outcomes. Permanent race tests verify literal JSON round-trip, escaped terminal controls, protocol capability agreement and rejection of malformed/duplicate/unknown/oversized input. Example documentation now distinguishes MCP service integrations from Hand-specific extension interfaces. Evidence is recorded in `tool-viewer-integration.json`; automatic tool-event presentation and the installed host journey remain pending.

### 2026-09-09 — Native example packager and installed viewer RPC

Added `scripts/package_extension_example.py` to build any of the four shipped native examples, create the capability/content manifest and obtain the pin from Hand's real inspector without executing the extension. Packaged tool-viewer successfully. The permanent `check_tool_viewer_rpc.py` installs that pin and exercises the real extension through built Hand RPC, verifying typed text/list/code presentation, exact JSON round-trip with escaped terminal controls, invalid record rejection and joined snapshot cleanup. No provider calls. `tool-viewer-rpc.json` records package/binary/script hashes and raw RPC evidence. Automatic tool-event subscription and final platform qualification remain pending.

### 2026-09-09 — Committed session lifecycle notifications

Extension activation now announces the selected session. New/resume/fork operations observe actual committed session-ID changes after releasing the session mutex and before releasing idle application ownership. Notifications are ordered close/open, share a two-second observation budget, and cannot undo a committed selection. Close-event callbacks are refused so they cannot act on the replacement session; questions are unavailable during session notifications. A real peer regression verifies seven exact events/IDs across activation/new/resume/fork and no events for failed or unchanged selections. Scoped race tests passed. See `extension-sessions-integration.json`. Shutdown close delivery, compaction notifications and final qualification remain pending.

### 2026-09-09 — Bounded extension shutdown notification

Host close now runs once, stops question handling, attempts the announced session's close notification within the existing two-second observation budget, and joins manager/process cleanup. Repeated close returns the same result without duplicate notification; new commands and reloads refuse a closing host. Retired hosts do not receive later session transitions. The real peer journey verifies transition plus shutdown ordering, and a deliberately hanging close observer is joined with snapshots removed inside four seconds. Scoped race tests passed. Evidence is recorded in `extension-shutdown-integration.json`; compaction notification and final qualification remain pending.

### 2026-09-09 — Committed compaction lifecycle observer

Harness now provides `OnCommitted` metadata observation after successful compaction commits and releases its session lock. Observation uses a two-second context with panic containment and emits nothing for skipped/failed compaction. Hand composes this into `context.compacted` delivery with session ID, reason and counts; callbacks are refused during observation and activation rejects in-flight compaction. A real extension process receives the successful manual compaction notification; existing lifecycle/session tests passed. Harness success/panic/skip/failure tests and compaction vet passed. An initial no-test run caused by an interrupted edit is retained but not counted as qualification. Evidence is in `compaction-observer-integration.json` and `harness-compaction-observer.json`. Automatic/background integrated qualification and final acceptance remain pending.

### 2026-09-09 — Joined background compaction observation

Expanded the real extension compaction journey to manual and background/preventive execution, checking the reported reason, session identity and committed count. Added a Harness concurrency regression that blocks the observer, proves the background done channel remains open and in-flight ownership remains visible, then releases it and joins the successful result. All focused race tests passed. Cumulative source/evidence manifests are `compaction-background-integration.json` and `harness-compaction-background.json`. Runtime automatic-trigger end-to-end and final candidate/platform qualification remain pending.

### 2026-09-09 — Extension host file reads

Added `file.read` dispatch through the registered `read_file` tool with host before/after hooks and permission checking. Callback payloads are strict, text responses are bounded to 64 KiB, images are refused and hook/tool panics are contained. The existing connection layer still requires the file.read capability. Policy observers cannot recursively request resources; a requesting extension that also declares policy.check is explicitly refused on this path. Focused race tests cover denied reads without execution, rooted actual file reading, path escape, malformed input and panic containment. Evidence is in `extension-read-integration.json`. Real-peer callback qualification, policy-extension resource composition, other resource callbacks and final acceptance remain pending.

### 2026-09-09 — Real file-read callback and policy composition

Added a real extension subprocess fixture that requests file.read and reports the actual callback response. Scenarios verify allowed rooted content, host approval denial, missing-capability refusal before host dispatch, and a separate mandatory tool-policy extension veto. The two-peer policy case completes without recursive deadlock. Every case checks joined shutdown and empty snapshot storage. Scoped race tests passed. `extension-read-process-integration.json` records source and raw results. The explicit restriction on policy extensions requesting their own host resources remains; other resource methods and final qualification are pending.

### 2026-09-09 — Reproducible cumulative Harness review bundle

Archived the cumulative Harness patch against historical base 09c30fe, with all 294 reconstructed files verified against source hashes and executable modes. Source and original index were unchanged; its fingerprint matches the latest 1,073-record native Harness suite. Reconstructed-tree vet passed. Fixed the evaluation checkout index stat cache so indexed patches apply immediately after raw blob materialisation; the permanent regression and all 94 Python runner tests passed. The patch inventory explicitly includes tracked files that match ignore rules, avoiding four documentation omissions discovered during reconstruction. See `harness-review-20260909.json` and the refreshed `harness-current-release.md`. Remote refs, native hosted qualification, review, publication permission, released dependency integration and final Hand acceptance remain pending.

### 2026-09-09 — Offline candidate patch verification

The pinned baseline runner now also accepts a reviewed candidate patch, archives its exact bytes/hash, applies it against the Git index, records the candidate tree and installs the independent fixtures afterwards. It preserves malformed-patch errors, refuses fixture replacement and unsupported link entries, and distinguishes candidate test results from task success. Six focused tests and all 97 Python runner tests passed. Both actual Harness calibration tasks passed with their historical fixing patch; initial failures caused by a missing explicit module-cache path are preserved alongside the corrected runs. Evidence: `evaluation-candidate.json`. No model runs occurred. Comparative execution, isolated untrusted candidates, cost enforcement, the full corpus and acceptance review remain pending.

### 2026-09-09 — Detect acceptance fixture mutation during candidate verification

Reproduced a false pass with real Go candidate code that rewrites its oracle while returning the expected value. The runner now checks fixture hashes after every command without following links, retains test and integrity outcomes separately, fails the overall result on mutation and stops subsequent commands. Added regression coverage for altered/deleted fixtures, leaf and parent symlinks, FIFOs and directories. All 100 Python runner tests passed; both real Harness calibration candidates passed with unchanged fixture hashes. `evaluation-fixture-integrity.json` retains failing and corrected evidence. Transient restored changes, forged output and hostile host code still require independent isolation/review; M8 and final acceptance remain unfinished.

### 2026-09-09 — Extension resource failure and uncertainty audit

Verified the existing file-write, network-fetch and process-run callbacks alongside read, lifecycle and context paths. Added 24 post-effect error/limit/panic cases across four resource methods, asserting exact error codes, preserved actual side effects, one invocation and after-hook behaviour. Mutating operations consistently report mutation_outcome_unknown after an effect followed by failure; read operations retain ordinary failure codes. The combined uncached race suite passed 67 test/subtest records with zero failures/skips, including explicit public HTTP fetch. Resource dispatch file statement coverage is 94.05%; this is not the full critical-group coverage. The initial missing fixture skip and test-helper compilation error are retained. See `extension-resource-failures.json`; final platform/candidate/scenario qualification remains pending.

### 2026-09-09 — Session operations reject unavailable runtime without mutation

Reproduced nil-runtime panics across new/resume/fork/export/rename/list/tree/select/usage and history access when a catalogue exists. These paths now return unavailable errors (or empty history), preserve catalogue state, create no export and release operation ownership. The regression checks actual catalogue snapshots and reacquires ownership after every rejected operation. App/controller/session lifecycle race tests passed 18 records; RPC/sessionio integration tests passed 56 records, with no failures/skips. Reproduced failures and corrected evidence are retained in `session-unavailable.json`. Final candidate and complete persistence qualification remain pending.

### 2026-09-09 — Session cancellation after ownership admission

A deterministic lock-contention regression reproduced successful results after parent-context cancellation for resume-current and empty session-tree reads. Added cancellation checks immediately after controller locking on these paths. Seven operations now verify cancellation after ownership admission, unchanged catalogue/selection and released ownership; the race test passed 20 repetitions (160 test/subtest records). Broader app/RPC/sessionio session regressions passed 81 records without failure or skips. `session-cancellation-admission.json` preserves the failing cases and corrected raw evidence. This does not interrupt already committed writes or change Service.Cancel semantics for idle administrative operations; final qualification remains pending.

### 2026-09-09 — Durable-session scenario evidence audit

Mapped all nine M3.1 scenarios to individually verified current test records and explicit remaining integrated/platform evidence. Fresh full session-package race suites passed 77 Hand and 172 Harness records with zero failures/skips; source fingerprints were unchanged throughout. The audit distinguishes real EFBIG write-limit evidence from an unproven ENOSPC/full-disk scenario, storage-level branch tests from built-Hand journeys, and separate usage/attachment checks from a combined export/restart journey. See `session-scenario-audit.md` and `.json`. All nine have development evidence; none is falsely marked final-candidate qualified.

### 2026-09-09 — Combined attachment and usage export/restart journey

Expanded the permanent attachment integration test to include reported/cache token usage, an unknown cancelled attempt, fork accounting separation, standalone export, deduplicated request replay and continued accounting across a second reopen. The 12 MiB attachment survives after the original blob store is moved away; unreferenced blobs remain excluded. Race test passed. `combined-session-export.json` records exact totals and evidence; the M3.1 audit now closes the separate-versus-combined storage evidence gap while retaining client-visible totals/rendering and final platform/candidate qualification as pending.

### 2026-09-09 — Built-client combined session export round trip

Expanded the real binary export journey with a 2 MiB attachment, reported and unknown usage, a separate home seeded from export only, explicit catalogue reconciliation, original-store removal and a second Hand process re-export. The journal is byte-identical; image bytes and exact accounting survive reopen. Initial missing-catalogue and ordinary-permission CopyFS fixture failures are retained; setup now explicitly reconstructs metadata and restores required private attachment modes. The corrected binary/invocation race test scope passed 4 records without failures/skips. `binary-combined-export.json` states the API import/setup and non-race child-binary boundaries. Final platform/candidate and UI rendering qualification remain pending.

### 2026-09-09 — Real filesystem exhaustion through built Hand export

Added Linux-only `TestBinaryExportReportsRealENOSPC`, requiring an explicitly supplied tmpfs capped at 8 MiB and a built Hand executable. Ran it in a read-only, network-disabled Docker container with a 2 MiB tmpfs. Kernel ENOSPC produced visible CLI exit 5/no-space diagnostic; source bytes and catalogue selection were unchanged, no partial output remained, and freeing space allowed a byte-identical retry. `session-enospc.json` binds the test, binary hashes, image and raw result. This is actual filesystem exhaustion rather than EFBIG injection; active conversation write/UI and final platform/candidate gates remain open.

### 2026-09-09 — Active conversation persistence under real ENOSPC

Added a local-provider built-CLI test that exhausts the bounded tmpfs after the prompt reaches the provider, then streams a large response. Hand reports exit 5/no-space rather than success, preserves the original durable prefix, and normal reopen after freeing space recovers both questions and allows new writes. The export and conversation cases passed together in a read-only, external-network-disabled Linux container. `conversation-enospc.json` binds binaries, source, image and raw output; reproduction instructions now run both cases. No model service was called. TUI visibility, native macOS and final candidate/dependency qualification remain pending.

### 2026-09-09 — Reject ambiguous durable usage accounting

Reproduced usage understatement after restart: duplicate/escaped/case-aliased input token keys changed 100 to 1, while missing/null counters and ambiguous origin keys were silently accepted. Version-1 usage records now require canonical unique schema keys, explicit request fields and non-null reported input/output counts; unavailable usage retains its explicit null representation. Twelve permanent corruption cases prove reader rejection, append refusal and unchanged journal bytes after reopening. Full sessionio race suite passed 90 records; app/RPC/CLI usage-budget-export-fork integration passed 62 records without failures/skips. `usage-integrity.json` retains before/after evidence. Final qualification remains pending.

### 2026-09-09 — Full native Hand regression and coverage refresh

Rebuilt the Linux worker and extension-peer fixtures, then ran the whole Hand tree uncached with race detection, cross-package coverage and all explicit local container/public-fetch fixtures. All 1,490 test/subtest records and 26 packages passed with zero failures/skips; both Hand and Harness source fingerprints stayed unchanged during the run. Hand coverage is 80.4099% (12,593/15,661 statements). Critical observed coverage ranges from 79.70% to 83.04%, still below the unchanged 90% threshold. Changed-source coverage is 80.2694%, with platform profile gaps retained. `current-native-suite.json` and `current-critical-coverage.json` now point to the fresh evidence; old runs are preserved. This is macOS development qualification using local Harness, not final-candidate/released-dependency completion.

### 2026-09-09 — Shared budget journal ambiguity blocks admission

Reproduced saved session/logical-run token and cost ceiling increases and deadline extensions through duplicate or case-aliased JSON fields. Harness now validates the complete bounded structure before typed decoding, rejecting duplicates, case aliases and null scalar values while preserving optional pointer compatibility. Sixteen reopened-journal cases assert refusal and byte preservation; nested price/quote cases cover the same ambiguity below the event envelope. Budget/runtime race suite passed 383 records, Hand integration 82, and the corrected null-price focused rerun 6; no failures/skips. `harness-budget-json.json` archives current source files and raw evidence. The prior cumulative release bundle and full coverage are explicitly marked stale pending refresh; no release occurred.

### 2026-09-09 — Missing budget fields cannot manufacture zero charges

Reproduced a reported settlement without `tokens` releasing a 20-token reservation to zero after reopen. Shared Harness budget decoding now recursively requires fields always emitted by the version-1 writer, retaining explicit-zero and omitempty compatibility. The restart regression verifies refusal of further admission and byte preservation; six nested quote/price omissions are rejected. Budget/runtime race scope passed 384 records, Hand integration 82, and focused final cases 8, without failures/skips. `budget-required-fields.json` archives source and before/after evidence. Full coverage/release bundle and final qualification remain pending.

### 2026-09-09 — Budget/usage fuzzing and refreshed Harness review candidate

Added bounded permanent budget and usage JSON fuzz targets. Final 60-second runs completed 1228990 budget and 1630440 usage executions without failure; budget round-trip comparison uses canonical serialized semantics and includes an explicit timezone-offset seed. Fresh full Harness race/coverage suite passed 1111 records across 32 packages, zero failures/skips, with unchanged source. Regenerated the cumulative patch and verified all 297 reconstructed files/modes plus vet; its fingerprint matches the full suite. See `harness-current-native-suite.json`, `harness-current-review.json` and refreshed release guide. Hand full regression, native release qualification, publication permission and final acceptance remain pending.

### 2026-09-09 — Full Hand integration against strict Harness budgets

Rebuilt container peers/workers and reran the entire Hand race/coverage suite against the current Harness source. All 1495 test/subtest records across 26 packages passed with no failures/skips and unchanged source trees. Refreshed both full profiles now share the same Harness fingerprint; current critical observations are no longer based on a stale Harness profile. Rebuilt Linux Hand also passed both bounded real-ENOSPC scenarios. Hand coverage is 80.4227%, changed coverage 80.2827%; thresholds and platform gaps remain explicit. See `budget-native-suite.json`. Release permissions, clean candidate, native hosted qualification and live evaluation remain outstanding.

### 2026-09-09 — TUI verification command journey

Added a permanent real-process TUI dispatcher journey covering review without execution, exact single-use confirmation, saved evidence listing, later-edit stale assessment and reviewed deletion with external effects retained. Invalid syntax and unavailable checkpoint recovery paths assert visible diagnostics and released session ownership. The uncached full TUI race suite passed 292 test/subtest records with zero failures/skips. Review handler coverage is 94.7%; confirmation, saved-result check, list and delete handlers are 100% in this package profile. See `tui-verification-journey.json` for hashed raw events, coverage and test source. This is in-process command integration, not a built-terminal rendering qualification. Aggregate coverage profiles were not replaced by this narrower run; full acceptance remains pending.

### 2026-09-09 — Malformed replay cannot imply successful execution

Reproduced null tool output rendering as a success tick and null messages/calls/notes/compaction rendering empty records. Consolidated replay decoding into source blocks, removed unreachable duplicate decoding, and require object data both for replay and output-viewer admission. Five entry-type regressions preserve error visibility/order across resize and reject terminal-control injection. Before-fix regression failed six test/subtest records; corrected full TUI race suite passed 298 without failures/skips. An intermediate unused-import build failure is retained with raw before/after logs and source hashes in `replay-integrity.json`. Full aggregate profiles are explicitly stale after these production edits; this direct-history API evidence does not establish durable-loader or built-terminal qualification.

### 2026-09-09 — Reopened image attachments remain visible

Reproduced resumed user/tool-result images disappearing from the transcript despite preserved durable attachment references. Source blocks now retain only image counts and show singular/plural attachment labels without resolving blobs or exposing payloads/storage hashes. Permanent regression uses a real store close/reopen, verifies externalized references, transcript resize and legacy inline compatibility. Before-fix test failed; full TUI race suite passed 299 test/subtest records with zero failures/skips. Hashed evidence is in `replay-images.json`. This proves metadata visibility, not image rendering or blob health; final built-terminal/platform acceptance and aggregate coverage refresh remain pending.

### 2026-09-09 — Replayed failure and cancellation flags

Reproduced IsError/Aborted records without error text rendering a success tick. Transcript and full-output viewer now share an outcome projection that preserves supplied errors and uses explicit failed/cancelled labels when only flags exist. Permanent tests cover each flag combination, explicit reasons, partial diagnostic output and resize. Before-fix run had four failed and two passed records; full TUI race suite now passes 305 records with no failures/skips. `replay-outcome.json` archives raw results and source hashes. Direct-history projection does not prove provider emission or built-terminal qualification; aggregate profiles remain pending refresh.

### 2026-09-09 — Fresh built-terminal verification journey

Rebuilt Hand and strengthened the real PTY verification runner with disabled terminal echo and an external command-execution marker. Initial batched text/Enter input stalled at the prompt; preserved that failure and separated typing from Enter. Corrected run passed review without execution, exactly one confirmed execution, saved evidence hash, later-edit stale status, reviewed deletion, retained snapshots and unchanged user edit, then exited zero. `verification-pty-refresh.json` binds the binary, runner, raw terminal and fixture artifacts. This closes the immediate in-process-only evidence gap for this workflow, but clean candidate, released Harness, other journeys and Linux qualification remain pending. No provider calls were made.

### 2026-09-09 — Structured state from terminal through RPC restart

Updated the structured-state PTY runner to disable echo and separate command typing from Enter. Fresh built-terminal journey completed eight commands, asserting invalid input/inspection did not mutate state and exactly five durable revisions retained the intended objective, decision and evidence reference. Extended the runner to launch a second Hand process through RPC using the same home/workspace; public context.state returned revision 5 and the exact three items. `context-pty-rpc.json` archives terminal, RPC transcript, journal and runner hashes. This is native macOS development evidence, not full coding-task parity or final candidate qualification. No model prompts or paid calls occurred.

### 2026-09-09 — Terminal budget decisions survive RPC restart

Strengthened the run-budget PTY runner with disabled echo, separate Enter delivery and a second built Hand process inspecting the same session through RPC. Seven terminal commands passed, including rejection of zero tokens, explicit 100000-token ceiling, exact USD 1.000000001 strict cost ceiling, 600-second deadline and persistent run selection. RPC recovered the exact ceilings/strictness/selection and original deadline; inspection left session journal bytes unchanged. `budget-pty-rpc.json` archives raw terminal, RPC transcript, journal and runner hashes. No prompts, provider calls or spending occurred. This does not qualify model admission or crash recovery; final candidate, released Harness and native platform gates remain pending.

### 2026-09-09 — Full regression after replay and terminal improvements

Fresh uncached full Hand race/coverage suite passed 1524 test/subtest records across 26 packages, zero failures/skips, in 164.12 seconds. Both source inventories stayed unchanged; Harness fingerprint remains 94d7b04316cab30fdf614e2f6db6bc6328091be5082cca562b51688c600487f9, matching its current full suite. Hand coverage is 81.0333% (12689/15659 statements), changed coverage 80.7964%; critical groups range 79.84–83.22%, below 90%. Initial run failed because ten archived Go source snapshots were accidentally discoverable packages; renamed only archive copies to .go.txt, preserving content hashes and updating artifact paths. Failed and corrected runs are preserved in `replay-full-suite.json`; current summaries now reference corrected evidence. Platform-profile gaps, clean candidate, released Harness and live evaluation remain open.

### 2026-09-09 — SDK event integrity

Reproduced duplicate/escaped/case-aliased event identity and outcome fields overwriting earlier values. SDK now rejects ambiguous event envelopes and known payload aliases while retaining unknown event kinds/extensions. Regression tests cover direct event decoding and completed execution snapshots. Full SDK race suite with explicit local HTTP/container fixtures passed 30 records, zero failures/skips. Initial sandbox failure and intermediate two skipped container cases are retained. `sdk-event-integrity.json` binds raw results, source archives and fixture environment. Aggregate coverage marked stale; outer execution/page schema audit and final qualification remain open.

### 2026-09-09 — SDK execution identity and lifecycle integrity

Reproduced conflicting execution states/identities accepted through duplicate, escaped or case-aliased fields. Execution decoding now validates its object before typed conversion, preserving supported pending/uncertain/accepted snapshots and additive future fields. Five negative cases failed before the fix; full SDK race suite with local HTTP/container fixtures now passed 36 records without failures/skips. `sdk-execution-integrity.json` retains raw before/after results and source archives. Event-page schema audit, aggregate coverage refresh and final qualification remain pending.

### 2026-09-09 — SDK event page ambiguity

Reproduced duplicate/aliased gap, next, events and item cursor fields silently replacing earlier values through public PollEvents and framed transport. Page envelope and each cursor item now validate canonical known names and unique decoded keys before typed conversion. Full SDK race suite with local HTTP/container fixtures passed 43 records without failures/skips. `sdk-page-integrity.json` retains before/after events and source archives. Omitted/null field semantics and aggregate/final qualification remain pending.

### 2026-09-09 — Event-page loss metadata cannot default silently

Reproduced missing/null gap and cursor metadata accepted as an empty zero-loss page. PollEvents now requires events/next/latest/gap fields and rejects null scalar metadata, retaining the server-compatible null event-list representation. Permanent transport tests assert invalid rejection and both valid empty-list forms. Full SDK race suite with explicit local HTTP/container fixtures passed 50 records without failures/skips. `sdk-page-fields.json` preserves before/after evidence and source hashes; aggregate coverage and final qualification remain pending.

### 2026-09-09 — SDK snapshot parser fuzzing

Added bounded SDK event/execution fuzzing seeded with valid lifecycle records and ambiguous identity/outcome examples. Accepted events round-trip through canonical JSON while permitting whitespace normalization; execution round trips preserve identity/lifecycle and terminal availability. Both initial and refined 60-second runs passed. Final progress: ('1m1s', '824275'). Raw output and permanent target source are archived in `sdk-snapshot-fuzz.json`. This supplements other parser targets; full coverage and final candidate qualification remain pending.

### 2026-09-09 — SDK submission response identity

Reproduced Submit accepting a valid execution record for another request even though the outer response echoed the submitted ID. Submit now binds decoded execution ID to the immutable prompt ID, matching Lookup identity enforcement. Framed transport regressions cover mismatched and matching pending/accepted/uncertain records. Full SDK race suite with explicit fixtures passed 64 records without failures/skips. `sdk-submit-identity.json` preserves before/after and source evidence. Final qualification remains open.

### 2026-09-09 — SDK terminal outcome contract

Reproduced acceptance of missing/unknown terminal status, null reason/verification and verified cancellation. SDK terminal events now require a known outcome, nonempty reason and explicit verification boolean; only completed can be verified. Existing synthetic terminal fixtures updated to include the server-required verified flag. Permanent negative and all-five-status positive checks pass; full SDK race suite with explicit fixtures passed 65 records without failures/skips. `sdk-terminal-outcome.json` retains before/after evidence. Historical/nonconforming version-1 terminal snapshots without verified now fail explicitly; nonterminal future events remain supported. Aggregate/fuzz refresh and final qualification remain pending.

### 2026-09-09 — Strict SDK snapshot fuzz refresh and client guidance

Refreshed the 60-second SDK snapshot fuzz run against strict terminal outcome validation; final progress ('1m1s', '483690'), no failure. `sdk-terminal-fuzz.json` binds current decoder/target and raw output. Expanded `sdk-snapshots.md` with rejection rules, explicit verification compatibility, page null semantics and original-ID reconciliation after invalid responses. Final aggregate/platform/released-candidate qualification remains pending.

### 2026-09-09 — RPC/SDK acceptance scenario reconciliation

Mapped all nine M4.1 and five M4.2 scenarios to inspected source tests/runners, recording exactly what each establishes and what remains unproven. `rpc-sdk-scenario-audit.json` hashes source pointers; companion Markdown lists gaps. No scenario promoted to qualified. Main newly isolated gap: retention and slow-reader timeout tests are separate, lacking one combined reconnect/exact-terminal recovery oracle. Recent SDK integrity evidence is now linked to M4.2 as well as M4.1. Released-package, full built-client, native cleanup and final-candidate gates remain open.

### 2026-09-09 — Combined slow-consumer terminal recovery

Added one framed-transport regression spanning actual file side effect, 400 progress events/retention gap, unread durable-result response, write timeout/join, ledger close/reopen, exact terminal recovery and duplicate prompt replay with no second effect. Passed 20 consecutive race runs; full RPC race suite passed 134 records with no failures/skips. `slow-consumer-recovery.json` archives results and source; RPC/SDK audit now distinguishes this completed development oracle from remaining built-process/native final qualification.

### 2026-09-09 — Linux RPC recovery and full suite

Ran combined slow-consumer recovery 20 times on Linux arm64. Broader run exposed a disconnect-test startup race and missing Go toolchain for extension fixture compilation. Added explicit active-backend wait and optional absolute HAND_TEST_RPC_NOTE_BINARY fixture. Retained a second setup failure caused by noexec tmpfs; reviewed extension snapshots require executable temporary storage. Corrected network-disabled/read-only Linux container full RPC suite passed 134 records with zero failures/skips; corrected macOS disconnect test passed 20 race repetitions. `rpc-linux-recovery.json` binds binaries, image, raw results and source archives. Linux run is not race-instrumented and does not replace built CLI/final candidate qualification.

### 2026-09-09 — Reusable Linux RPC qualification runner

Added check_rpc_linux.py to build pinned-source fixtures, require an immutable Docker image, inventory expected tests inside Linux, reject missing/skipped/failed tests, execute 20 slow-consumer recovery repetitions and bind before/after source fingerprints. Unique named containers are cleaned up on timeout. Actual runner execution passed all 134 full-suite records and 20 repetitions with unchanged sources. `rpc-linux-runner.json` archives logs/report and `rpc-linux-runner.md` documents reproduction and non-race/final-candidate limits.

### 2026-09-09 — Linux runner binds actual Harness inputs

Extended the Linux RPC runner to resolve actual Harness module metadata, fingerprint dependency source before/after and record Go version/architecture/CGO/workspace settings. Revalidated runner: 134 test/subtest records and 20 recovery repetitions passed, with both Hand and Harness unchanged. `rpc-linux-bound.json` records the local workspace dependency explicitly; this is not a released Harness qualification.

### 2026-09-09 — Dependency fingerprint regressions

Added three permanent acceptance-runner tests proving byte edits, executable mode changes, deletion, newly nested source and changed symlink targets alter dependency fingerprints without requiring Git; Git metadata alone does not. New tests and six existing source-evidence tests passed. `rpc-fingerprint-tests.json` archives runner-test evidence, explicitly not product scenario qualification.

### 2026-09-09 — Built RPC oversized-frame admission boundary

Added a built Hand regression that negotiates RPC, sends an oversized valid JSON prompt followed by another prompt, then asserts stream closure/nonzero exit, bounded stderr size diagnostic, no stdout contamination, zero local-provider calls and an empty durable request ledger. Both initial provider sentinel and strengthened ledger runs passed. `rpc-frame-binary.json` archives source and raw test output; audit updated without claiming final native/released-candidate completion.

### 2026-09-09 — Built framing rejection and connection recovery

Extended the built Hand framing journey with unsupported protocol version, duplicate envelope ID and prompt-before-hello rejection. It checks bounded JSON error/code/identity, successful later negotiation on the same connection, then fatal oversize closure. Final zero-provider-call and empty-ledger assertions cover every rejected prompt including the trailing request. Focused race harness passed; child is a fresh non-race binary. `rpc-framing-recovery.json` records source/raw evidence. Final native/released-candidate qualification remains pending.

## Linux built-client framing qualification

The source-bound Linux RPC runner now builds Hand and its command test binary,
then runs the real framed-client rejection regression in addition to the RPC
package and 20 slow-consumer recovery repetitions. The complete runner passed
134 RPC test/subtest records, all 20 repetitions, and the built-client check.
Both Hand and Harness source inventories stayed unchanged during execution.
Evidence: `rpc-linux-framing.json`. This remains development Linux arm64
evidence without race instrumentation or a released Harness dependency.

## Unattended RPC cold-start refusal

Added `scripts/check_rpc_cold_start.py`. Native macOS built Hand refuses legacy
workspace grants through explicit RPC approval, handles denial and cancellation
across distinct processes/sessions, and leaves the target absent, legacy settings
unchanged and workspace untrusted. Three local provider requests; no paid calls.
Initial fixture reused a durable session.new request ID; failure preserved and
fixture corrected with unique IDs and a distinct-session assertion. Evidence:
`rpc-cold-start.json`. Linux and final candidate qualification remain open.

## Linux unattended RPC cold-start

The same cold-start denial/restart/cancellation runner passed with the Linux
arm64 Hand binary in a network-disabled Python container. Three loopback fixture
requests, no target-file writes or implicit workspace trust. Recorded binary
hash matches the earlier source-bound Linux build. Raw exchanges, provider
requests and container command retained in `rpc-cold-linux.json`. Final clean
candidate and released Harness qualification remain pending.

## Full Hand suite after SDK hardening

Uncached full native macOS race/coverage suite passed 1569 test/subtest
records across 26 packages with zero failures/skips. Both source trees
remained unchanged during the 173.02 second run. Overall coverage 81.11%;
changed production coverage 80.87%, with platform profile gaps.
Critical coverage refreshed against unchanged Harness evidence; thresholds
remain unmet. See `sdk-full-suite.json`; no final acceptance claimed.

## Reject ambiguous permission journals

Real journal-reopen regressions exposed accepted duplicate/version/scope fields,
case aliases, null metadata and omitted fields. Added strict canonical JSON
validation before replay publishes any authority. Nine regression records failed
before the fix (including parent); the full permissions race suite now passes
47 test/subtest records without failures/skips. Tests retain corrupted bytes,
verify released ownership, and reopen the authentic journal successfully.
Evidence: `authority-json.json`. Aggregate coverage marked stale pending refresh.

## Authority storage platform checks

Added real filesystem regressions for public directory/file modes, symlink and
directory substitution, journal/file size bounds, malformed JSON and trailing
JSON. All assert unchanged evidence and successful authentic repair/reopen.
Corrected fixture slice aliasing found by Linux; failed log retained. Full
permissions package now passes 56 test/subtest records on macOS with race
instrumentation and 56 on Linux arm64 without it, zero skips/failures. See
`authority-storage.json`. Final qualification and coverage gates remain open.

## Permission migration and recovery guide

Added `docs/permission-migration.md` and executed its inspection, acknowledgement,
revocation, restart and quarantine-recovery sequence through a fresh built CLI
in an isolated HOME. Added exact authority directory to startup failures and a
regression preserving corrupt bytes. Evidence: `permission-migration.json`.
This advances permission-specific M8.2 documentation; full release/session
migration and rollback qualification remain pending.

## Authority parser fuzz qualification

Added permanent fuzz target for JSON validity, stable typed round trips and
duplicate decoded version keys, including escaped aliases. Package race suite
passed 63 test/subtest/seed records without failures/skips. Timed native
fuzz run passed 1460064 executions in 60.01 seconds.
Evidence: `authority-fuzz.json`; final candidate and broader gates remain open.

## Scoped policy acceptance reconciliation

Mapped all nine M5.1 scenarios to inspected source assertions and explicit
remaining proof. Corrected the primary requirement on recent authority/migration
evidence from M1.3 to M5.1; retained supplementary historical links. Routine-work
prompt counts currently have only hook evidence, and interrupted legacy-import
recovery still needs a journey. No scenario or milestone is finally qualified.
See `scoped-policy-scenario-audit.json`.

## Partial legacy import resume

Extended the real CLI permission migration journey to three grants. After
restoring authentic one- and two-record prefixes, inspection preserves the
partial journal and explicit acknowledgement appends only missing grants.
Repeated acknowledgement leaves recovered bytes unchanged. Both prefix cases,
revocation and quarantine recovery passed. Evidence: `permission-partial-import.json`.
This is durable-prefix recovery evidence, not a live process-kill experiment.

## Built-client routine permission counts

New RPC workflow executes two same-file writes and two identical shell checks
with exactly two explicit persistent prompts. Different file, expanded command,
network fetch and external read each prompt separately and are refused. Actual
file/check outputs and two exact grants verified on native macOS and Linux
arm64 container. Evidence: `permission-prompt-count.json`; policy documented in
`docs/permission-migration.md`. Final candidate qualification remains open.

## Permission operation/lifetime matrix

Added eight operation types across invocation/session/persistent lifetimes, with
six identity contexts and explicit operation/resource mismatch and revocation
checks. Full permissions race suite passed 88 test/subtest/seed records, zero
failures/skips. Package-only coverage is 82.94%, distinct from the mapped critical
group. Evidence: `permission-lifetime-matrix.json`; final platform/candidate
qualification remains open.

## RPC usage certainty correction

Recent fixture traces exposed known=true alongside unknown request counts. Wire
encoding now reports known only for complete totals and adds reported_totals_known
for subtotal availability, preserving all counters and prior uncertainty. Five
wire cases plus persisted/cancelled accounting tests pass. Built nine-request
workflow confirms all nine unknown requests yield known=false without invented
tokens. Evidence: `wire-usage-certainty.json`. Historical durable replay retains
original encoding; compatibility guidance added to protocol v1.

## Full suite after authority and RPC usage changes

Uncached native macOS race/coverage suite passed 1627 test/subtest/seed
records across 26 packages without failures/skips in 173.56 seconds.
Hand and Harness source inventories remained unchanged during execution.
Overall coverage 81.17%; changed production 80.94%.
Critical metrics refreshed; required thresholds and platform profile gaps remain
open. Evidence: `authority-wire-full-suite.json`.

## Actual legacy importer interruption

Added `scripts/check_permission_interrupt.py`. Actual CLI importer stopped/killed
after partial journal creation: one surviving grant on macOS and six on Linux.
New CLI processes inspected and resumed each to 256 exact reviewed grants,
preserved the prefix, and repeated acknowledgement appended nothing. Evidence:
`permission-interruption.json`. One process-kill journey per platform; no power-loss
or exhaustive write-boundary claim. Final candidate/released Harness remain open.

## Invalid permission inputs

Added rejection cases for malformed grant identity/scope/lifetime, duplicate IDs,
empty/invalid command identities, credentialed/path/query/non-HTTP network origins,
MCP shape and invalid workspace/digest. Rejection preserves existing valid grants.
Full package race suite passes 112 test/subtest/seed records with no failures/skips;
package-only coverage rises from 82.94% to 86.45%. This is not the critical-group
metric or final qualification. Evidence: `permission-rejections.json`.

## Legacy settings startup rejection

Added directory/FIFO/symlink/oversize/malformed/invalid-tool startup cases.
Each preserves project settings, releases authority ownership on failure and
reopens with only an unapproved proposal after repair. Selected CLI authority
race suite passed ten test/subtest records, zero failures/skips. Evidence:
`legacy-settings-rejection.json`; final platform/candidate qualification remains.

## Linux full-suite toolchain prepared

Fetched official Go 1.25.1 Bookworm image after diagnosing a stalled Docker
credential helper. Used isolated empty Docker config; personal config unchanged.
Immutable image `sha256:c423747fbd96fd8f0b1102d947f51f9b266060217478e5f9bf86f145969562ee`
verified Linux arm64, Go 1.25.1, CGO enabled and GCC 12.2. Evidence:
`linux-toolchain.json`. This is environment preparation, not passing Linux tests.

## Full Linux suite setup and approval boundary

Implemented source-bound full Linux runner with pinned Go image, Linux Docker
CLI, native test volume, non-root process, explicit fixtures and 2 MiB ENOSPC
filesystem. Initial and corrected-environment runs retained in `linux-full-setup.json`.
Remaining nested-container failures are socket access: mounted socket is mode660
root:root. Auto-review rejected adding supplementary GID0 because it grants
root-equivalent Docker daemon control; explicit approval needed. No rejected
rerun or socket permission change occurred. Independent goal work remains.

## Preserve failed Linux suite results

Runner now summarises actual pass/fail/skip/package outcomes and unfinished
records even after nonzero Go exit, and captures source-after snapshots on failure.
Cleanup exceptions are recorded instead of preventing report output. Three
reporter unit tests pass; both prior failed raw runs reconciled without changing
their status. Evidence: `linux-failure-reporting.json`. Docker socket group
approval remains pending; no rejected action was retried.

## Permission preparation rejects absent commits

Regression exposed model/profile-switch panic when a permission callback returns
(nil, nil). Preparation now rejects the absent commit explicitly. Both missing
rebinder and nil-commit cases preserve model, provider and authority. Existing
valid/failed atomic preparation and binding-isolation checks also pass: nine
targeted race test records. Evidence: `permission-binding-failure.json`; full
coverage marked stale. No Docker socket escalation was attempted.

### Profile preparation cancellation regression — 2026-09-09

Added a permanent regression for cancellation during provider, identity and permission preparation. Each case preserves all runtime fields, active profile, permission profile, authority and service options, avoids the permission commit, and succeeds exactly once on fresh retry. The targeted race-enabled run passed 12 test/subtest records with no failures or skips. Evidence: `profile-cancellation.json`. Existing cancellation checks required no production change. Full candidate, coverage and platform gates remain open.

### Distribution wiring and upgrade guide — 2026-09-09

Connected make dist to the existing CLI/Linux-worker archive builder and enabled strict worker verification in both CI workflows. Four real archives passed checksum/content/worker checks; the native darwin-arm64 CLI passed credential-free smoke. Five worker-validator unit tests passed. Reusing the output directory failed without changing any existing artifact hash. Added installation, complete-data backup, session export and rollback instructions; corrected README claims that /new deletes history. Evidence: `dist-wiring.json`. Dirty-checkout development evidence only; no hosted CI, release publication or final qualification claimed.

### Strict archive integrity verification — 2026-09-09

Archive smoke validation now rejects duplicate/unexpected paths, missing or nonregular contents, nonexecutable CLIs and CLI OS/CPU mismatches. Native release identity requires an exact output match. Seven archive integrity unit methods and five existing worker methods passed, along with revalidation of all four development archives and the native darwin-arm64 smoke. Initial malformed link/FIFO fixture failures were retained and corrected. Evidence: `archive-integrity.json`; final release/platform gates remain open.

### Session and restore user guide — 2026-09-09

Added source-reviewed instructions for session navigation, branching/export, checkpoint capture, exact-preview confirmation, interrupted restore resolution and named verification. All 17 documented slash commands and three startup flags match current registration; relative links resolve and the JSON example parses. Evidence: `session-restore-guide.json`. This is documentation validation, not new runtime or final-candidate acceptance evidence.

### Automation and SDK user guide — 2026-09-09

Documented one-shot/RPC/embedded selection, lifecycle ownership, scoped authority opt-in, immutable prompt requests, durable lookup, event gaps, approvals, cancellation and close semantics. Corrected obsolete protocol claims that dispatch was unimplemented. Both checked-in Go examples compiled and exited zero in isolated directories without submitting model prompts. Evidence: `sdk-guide.json`; released-package and full-candidate acceptance remain pending.

### Package and extension guide — 2026-09-09

Added inspection, pinned installation, update/remove/rollback, runtime review, launch approval and example packaging instructions. Matched documented commands against CLI registration and checked all relative links. Native task-note package built and passed real CLI manifest/pin inspection without extension execution. Evidence: `extension-guide.json`. Final release and full activation qualification remain open.

### User guide coverage audit — 2026-09-09

Added troubleshooting for startup, scoped permissions, sessions, restores, containers, extension admission, RPC uncertainty and usage/budgets. Audited all required guide topics and checked their relative links. M8.2 is now in progress because distribution integration and substantial user documentation exist; it is not qualified complete. Evidence: `user-guide-audit.json`. Final candidate review, command reproduction and full acceptance remain open.

### Full Python runner/checker verification — 2026-09-09

Verified shared CI already discovers all Python runner and checker tests, including new archive checks. Ran both suites: 113 runner tests and 17 checker tests passed with no failures or skips. Retained raw output and source hashes in `all-python-tests.json`. These validate acceptance tooling, not product completion; final clean candidate and hosted CI remain pending.

### Tariff JSON ambiguity rejection — 2026-09-09

Confirmed seven duplicate/alias/null tariff inputs previously received approval digests. Added canonical unique-field validation for review envelopes and tariff objects, rejecting null supplied fields and invalid UTF-8 before typed validation. Targeted race suite passed 17 records after the fix; initial eight failed records retained. Rejection preserves input and valid replacement remains usable. Evidence: `price-ambiguity.json`. Full coverage marked stale; no budget gate completion claimed.

### Cost view restart and failure paths — 2026-09-09

Added durable-ledger tests for reserved retry, settled compaction and unpriced generation attempts through the Hand cost view. Restart preserves totals, category subtotal and uncertainty; rejected strict-mode conversion preserves history. Missing runtime/session, cancellation and busy errors return no usable view and release ownership. Six race-enabled test records passed. Evidence: `cost-view-restart.json`; no full coverage or final budget qualification claimed.

### Full native refresh after tariff hardening — 2026-09-09

The uncached native macOS race/coverage suite passed 1,681 test/subtest/seed records across 26 packages, with zero failures/skips, in 170.10 seconds. Hand and Harness source inventories were unchanged during the run. Overall coverage is 81.4201%; changed coverage 81.2018%, with six platform-only profile gaps. Critical-group coverage remains below 90% (permissions 89.4558%, budgets 80.5941%). Raw logs, profile and provenance are archived in `tariff-full-suite.json`; current native/critical reports refreshed. Dirty checkout, local unreleased Harness and incomplete cross-platform qualification remain explicit.

### Authority write-failure recovery — 2026-09-09

Added real closed-descriptor failures for grant and revocation after an existing durable grant. Both paths poison live authority and deny subsequent access; later mutations fail without changing journal bytes. Reopening preserves actual durable history and explicit revocation retry succeeds. Full permissions package passed 115 race-enabled records with no failures/skips. Evidence: `authority-write-failure.json`. No production change; broader persistence/platform qualification remains open.

### Tariff review parser fuzzing — 2026-09-09

Added a permanent six-seed fuzz target for valid JSON, duplicate decoded-field rejection and stable canonical review digests. Race-enabled regression/seed run passed 24 records; two-worker 60-second fuzz run passed 683809 executions (last elapsed 1m1s). Evidence: `price-fuzz.json`. Development parser evidence only; final candidate fuzz and complete budget qualification remain open.

### Shared RPC tariff validation — 2026-09-09

Budget audit found RPC still decoded tariff arrays permissively. Added exported app decoder shared with tariff-file canonical validation; RPC rejects duplicate/alias/null rate input before setting prices. Fourteen targeted RPC records passed, including replay/state-preservation and existing budget tests; 24 app regression/fuzz seed records passed. Initial malformed fixture and unused-import failures retained without claiming before-regression proof. Evidence: `rpc-price-ambiguity.json`. Full coverage now stale; budget scenario audit and top-level parameter ambiguity remain separate follow-up work.

### Canonical RPC confirmation fields — 2026-09-09

Confirmed duplicate/alias confirmation and duplicate limit fields previously reached budget mutation (five failed records across four cases). Added shared top-level schema validation before permissive Go decoding; escaped duplicate names are rejected. Repeated rejected request IDs preserve original decisions. Full RPC race package passed 143 records with no failures/skips. Evidence: `params-integrity.json`; nested schema and final candidate qualification remain separately scoped.

### Budget scenario reconciliation — 2026-09-09

Mapped all nine M6.3 scenarios to concrete Hand/Harness test assertions and source hashes, with development evidence and scenario-specific remaining proof. No scenario marked qualified. Explicit gaps include final built-client tariff/advisory journeys, fallback/cache traceability, cancellation trials, platform/crash evidence, released Harness and >=90% critical coverage. Audit: `budget-scenario-audit.json` and `.md`.

### Built budget refusal and advisory restart journey — 2026-09-09

Extended check_cost_rpc.py to reject missing/expired/unbounded tariffs before provider dispatch and to observe advisory unknown charges through process restart. Fresh built Hand passed four clean exits and exactly two local fixture requests. Replay made no extra provider call; strict conversion after historical unknown charge was rejected without changing the view. Evidence: `budget-advisory-journey.json`; paid calls zero, final candidate/platform qualification pending.

### Linux ARM64 monetary-budget journey — 2026-09-09

Fresh Linux binary passed strengthened budget journey in pinned Python image with network disabled, no Docker socket mount, read-only sources/binary and temporary evidence mount. Four Hand exits were clean and exactly two internal fixture requests occurred. Strict missing/expired/unbounded refusals, advisory restart, replay and rejected strict conversion all passed. Evidence: `budget-advisory-linux.json`. No paid calls; full platform/candidate qualification remains open.

### Canonical RPC parameter fuzzing — 2026-09-09

Added seven-seed fuzz target checking schema field spelling, stable control values and duplicate escaped/literal confirmation rejection. Race regression/seed run passed 13 records. Two-worker 60-second fuzz run passed 789016 executions, last elapsed 1m1s. Evidence: `params-fuzz.json`; final candidate/platform proof remains open.

### Repeated budget deadline enforcement — 2026-09-09

Ran 30 race-enabled repetitions of backend/controller provider deadline adapters and completion-validator deadline enforcement: 120 test/subtest records passed, no failures/skips. Provider stop and exhaustion cause, unverified expired completion, and refusal to auto-renew are asserted. Evidence: `deadline-repeat.json`. Does not substitute for UI cancellation-initiation p95 or actual child-process cleanup measurement.

### Manual compaction cancellation timing and keyboard quit — 2026-09-09

Measured 30 trials each for Ctrl+C, /quit, /exit and application close through the real compaction controller to provider context observation. Initial keyboard /quit trial exposed Enter being ignored during compaction; now /quit and /exit dispatch to existing cancel-and-join handling. Full TUI race package passed 310 records with no failures/skips. Combined latency tests recorded 450 trials across 15 paths, each p95 below 1000 ms. Transcript preservation and worker release asserted. Evidence: `compaction-cancellation-latency.json`; failures retained. Full coverage remains stale; this is development evidence, not final performance or process cleanup qualification.

### Keyboard quit during session/profile preparation — 2026-09-09

Four permanent keyboard regressions exposed /quit and /exit being ignored by busy Enter guards during session/profile changes (five failed test records). Both commands now reach existing cancel-and-join handling. Tests assert deferred actual QuitMsg, blocked new prompts, preserved UI identity/model and no cancelled provider commit. Full TUI race package passed 315 records, no failures/skips. Evidence: `change-keyboard-quit.json`. This is lifecycle ordering evidence, not cancellation latency or final candidate qualification; coverage remains stale.

### Session worker cancellation timing and stale results — 2026-09-09

Added 30 UI-to-worker context timing trials for Ctrl+C, /quit, /exit and close (120 total). All four p95 values below 1000 ms; cancellation cause, identity preservation, actual quit messages, duplicate result rejection during a fresh operation and successful reuse asserted. Selected race suite passed 14 records without failures/skips. Evidence: `session-cancellation-latency.json`. No production change. In-process dispatcher evidence only; filesystem/crash and final platform/candidate qualification remain open.

### Full native refresh after RPC and cancellation changes — 2026-09-09

Uncached macOS race/coverage run passed 1,723 test/subtest/seed records across 26 packages with zero failures/skips in 162.50 seconds. Hand and Harness inventories unchanged during measurement. Overall coverage 81.5261%; changed coverage 81.3132%, with six platform-only profile gaps. All critical groups remain below 90% (permissions 89.7959%, lifecycle 82.1581%, protocol 80.4577%). Raw coverage/logs and source provenance archived in `cancel-native-suite.json`; current reports refreshed. Initial sandbox-restricted run failed (1,411 passes, 15 failed records) on loopback/Docker/DNS access, preserved in `cancel-restricted-suite.json`. Native rerun approved independently, with no Linux group escalation. Dirty checkout, unreleased Harness and final acceptance gaps remain.

### Legacy permission failure oracles — 2026-09-09

Added malformed/partially decoded legacy trust/settings tests, failed trust-mark preservation, directory-read rejection and blocked-parent/target-directory save tests. Each asserts no usable grant/store or preservation of existing bytes. Full permissions race package passed 126 records without failures/skips. Combined development coverage with existing native/Harness profiles gives permissions group 91.3265%; separate from unchanged full-suite report. Evidence: `legacy-permission-failure.json`. No production change or final acceptance closure.

### RPC queue rejection invariants — 2026-09-09

Added twelve malformed/invalid control cases, each replayed with the same ID. Assert expected errors without result, unchanged steering/followup queue identity/order/content, zero backend calls or automatic marking, and successful fresh correction preserving unrelated work. Full RPC race package passed 164 records without failures/skips. Combined development protocol group coverage 80.9859%, below 90%. Evidence: `queue-rejection.json`. No production change or final acceptance closure.

### RPC journal write failures and restart — 2026-09-09

Added real closed-descriptor failures at intent, bind and completion writes. Twenty race-enabled repetitions each passed (80 test/subtest records). Failed writes poison owner, deny retry/new execution, preserve durable bytes, and reopen existing intent/accepted records as uncertain without completion. Unwritten intent is admitted once by fresh owner because execution was never granted. Evidence: `ledger-write-failure.json`. No production change; partial-write/fsync/crash and final-candidate qualification remain separately required.

### RPC emergency cancellation with failed request storage — 2026-09-09

Regression showed poisoned request journal blocked cancellation of active provider work. Valid cancel now signals existing application/extension/verification contexts when storage is unavailable, pauses automatic followups and returns ledger_failure without success. Healthy conflicts and malformed cancellation do not trigger fallback. New prompt/followup/queue work remains denied without side effects. Full RPC package passed 174 records; strengthened targets passed 120 records across 20 repetitions. Evidence: `rpc-journal-dispatch.json`. Original failure retained; coverage marked stale, final qualification open.

### Failed RPC terminal persistence with queued automatic work — 2026-09-09

Added 20 race-enabled repetitions of a gated backend finishing after the request journal descriptor fails. Each asserts exactly one backend call, unchanged automatic followup, state-reported persistence failure and pause, no retained terminal completion, released active ownership, unchanged disk bytes, denied resume, and uncertain/no-execute request replay after reopening. All 20 records passed. Evidence: `terminal-write-failure.json`. No production change; built-client/platform/crash and final-candidate gates remain open.

### RPC request journal object fuzzing — 2026-09-09

Added eight-seed permanent fuzz target for strict journal object decoding. Race seed suite passed; two-worker 60-second fuzz run passed 1341891 executions. Oracles cover valid UTF-8/JSON, standard-decoder field agreement, roundtrip value preservation, duplicate decoded-key rejection and trailing-object rejection. Evidence: `ledger-fuzz.json`. Object parsing only; identity/state transitions and storage crash guarantees remain separately tested. No production change or final acceptance closure.

### RPC rejection preserves charged budgets — 2026-09-09

Strengthened existing budget rejection suite with real session token/cost admissions: outstanding retry reservation and settled compaction, yielding 18 committed tokens and 20 nano-units with one uncertain attempt per ledger. Every rejection/replay now also asserts unchanged charged views and session entries, with no backend calls. Twenty race-enabled repetitions passed (180 test/subtest records). Evidence: `budget-charged-rejections.json`. In-memory session fixture; physical restart and run-scoped charged qualification remain separate. No production change or acceptance closure.

### Charged run budgets across session/journal reopen — 2026-09-09

Added disk-backed run-specific budget fixture with reserved retry and settled compaction. Rejected ceiling/currency/selection controls preserve exact views; clean close/reopen of session and RPC journal retains selection, charges, uncertainty and identical error replay. Explicit ceiling increase keeps prior usage. Twenty race-enabled repetitions passed. Evidence: `run-budget-charged-reopen.json`. Clean reopen only, not process crash/power loss; no paid calls, production changes or final acceptance closure.

### Verification subprocess cancellation with failed RPC journal — 2026-09-09

Extended real named-verification journey with journal write failure after subprocess startup. Twenty race-enabled repetitions passed (40 records); every recorded child PID gone within five seconds (maximum 12.931 ms). Cancellation reports ledger_failure and paused automatic followups; reopened request remains uncertain and replay does not start another worker. Evidence: `verification-journal-cancel.json`. Single native exec child, not complete platform/descendant qualification. No production change or final acceptance closure.

### Extension command cancellation with failed RPC journal — 2026-09-09

Extended built task-note extension journey with journal failure while awaiting interactive question. Twenty race-enabled repetitions passed (40 records), asserting command join, no pending question, unchanged state revision, paused automatic followups and disclosed storage error. Reopened request remains uncertain and does not dispatch again. Maximum join plus state-check measurement 2.4501 ms. Evidence: `extension-journal-cancel.json`. Command cancellation only, not host process termination; no final candidate/platform closure.

### Full native journal-hardening refresh and vet fix — 2026-09-09

Uncached native macOS race/coverage suite passed 1,770 records across 26 packages with zero failures/skips in 176.29 seconds; Hand/Harness inventories unchanged during run. Overall coverage 81.7907%, changed 81.5366% with six platform gaps. Permissions critical group 91.1565% now above 90% in this full run; other groups remain below threshold (protocol 81.9242%, budgets 81.3861%). Evidence: `journal-native-suite.json`. Build and formatting passed; vet found prior test copied runtime mutex. Replaced with explicit runtime field/identity invariants; four targeted race records and full vet passed afterward. Static evidence: `journal-static-checks.json`, preserving original vet failure. Test-only change follows full snapshot; no production change or final acceptance closure.

### Long-transcript benchmark with accepted input oracles — 2026-09-09

Strengthened benchmark to assert 10000 retained blocks, route keys through Model.Update, ensure each measured key changes input, avoid textarea capacity saturation and close every model. Thirty runs of 1000 events passed; combined p95 0.446375 ms, worst-run p95 0.467625 ms, maximum 1.394125 ms. Evidence: `transcript-typed-performance.json`. In-process measurement excludes terminal display/OS delivery; current CPU query denied by sandbox, final reference-hardware/candidate qualification remains open. No production change.

### Reproducible transcript performance runner — 2026-09-09

Added check_transcript_performance.py with fixed 30x1000 workload, fresh external output directory, raw command logs, toolchain/CPU/memory capture, Hand/Harness snapshots before/after and failure retention. Supports Git checkouts and released module directory inventories; five parser/provenance tests passed. Real run passed with unchanged sources on Apple M4 Max; p95 0.449792 ms. Evidence: `transcript-provenance.json`. Dirty development snapshot remains distinct from final candidate/reference qualification.

### Third calibrated evaluation task, second repository — 2026-09-09

Added hashed large-file search oracle pinned to Hand e6dd263. Independent baseline checkout fails actual match assertions; independent candidate checkout with minimal streaming-preserving fix passes. Oracle also checks exact lines, filename filter and result cap; fixture integrity verified by baseline runner. Catalogue now three Go bug-fix calibration tasks across Hand/Harness, zero held-out tasks and zero paid runs. Evidence: `evaluation-search-calibration.json`. Full diverse corpus, pins, spend authority and comparative evaluation remain outstanding.

### Evaluation tooling regression refresh — 2026-09-09

After adding Hand search calibration, all 66 evaluation runner/verifier tests passed with zero failures/skips. Actual catalogue qualification check correctly exits 1: fewer than 30 tasks/10 held-out/three repetitions, missing agent/model pins and comparison modes, incomplete categories. Raw output and source hashes in `evaluation-tooling-refresh.json`. Tooling fixtures do not count as product or live-agent evidence; no paid execution occurred.

### Optional MCP startup and baseline comparison — 2026-09-09

Added explicit optional-MCP probe mode while preserving original required-server expectations. Existing probe tests passed. All 120 fresh-process PTY trials (30 per case) reached rendered input and exited zero. No-MCP p95 88.2635 ms meets archived baseline x1.2+50 limit 116.6107 ms; fast/slow/failed optional MCP p95 86.6954/93.9097/92.5471 ms. Hardware/platform/toolchain/terminal match baseline. Evidence: `startup-optional-performance.json`. No model calls; dirty source/local Harness and final candidate qualification remain open.

### Unreadable session usage cannot retain prior-session totals — 2026-09-09

Added failing regression for attaching a session with unsupported usage annotations after a known charged session. SetController now clears cached totals and marks prior usage unknown while displaying the read failure; returning to a valid session recovers its totals. Full TUI race suite passed 321 records, zero failures/skips. Evidence: `usage-attachment-failure.json`, including before-fix failure and raw after log. Existing full coverage explicitly marked stale after this production change. No final acceptance closure.

### Persistent usage uncertainty and compaction read failure — 2026-09-09

New regressions confirmed the persistent status line hid incomplete accounting and compaction read failure retained a known-total flag. The status now labels recorded subtotals incomplete whenever history or attempts are unmeasured; compaction read failure preserves the known subtotal but marks it incomplete. Full TUI race suite passed 325 records, zero failures/skips. Evidence: `usage-status-uncertainty.json`, including before-fix failures. Full coverage remains stale; no final acceptance closure.

### Full native accounting refresh — 2026-09-09

Uncached macOS race/coverage suite passed 1,775 records across 26 packages, zero failures/skips, in 177.35 seconds. Hand and local Harness source inventories stayed unchanged during execution. Overall statement coverage 81.8554%; changed coverage 81.5881% with six unprofiled platform files. Accounting critical coverage 83.9152%, permissions 91.3265%; all other critical groups remain below 90%. Evidence: `accounting-native-suite.json`, with archived raw logs, profile, runner and source inventory. Current native/critical reports refreshed. Full candidate, released Harness, platform, live and acceptance-review gates remain open.

### Run journal identity/outcome integrity — 2026-09-09

Before-fix regressions confirmed ambiguous completion records were accepted and unavailable SessionRuns could panic. Added exact-field nested decoder rejecting duplicate/escaped/case-aliased/unknown fields and null or missing outcome verification data. Invalid history yields no run list and prevents later append without changing annotations. SessionRuns now returns unavailable error for absent runtime/session. Twenty race-enabled repetitions of run journal, availability and run budget suites passed 760 records, zero failures/skips; app vet passed. Evidence: `run-journal-integrity.json`. Full coverage marked stale; final acceptance remains open.

### Run journal parser fuzz qualification work — 2026-09-09

Added eight-seed permanent fuzz target with canonical roundtrip and duplicate identity/outcome/trailing-object rejection oracles. Race seed suite passed nine records. Two-worker 60-second run passed 799360 executions in 61.01 seconds. Evidence: `run-journal-fuzz.json`. Input limited to 64 KiB; decoder-only development evidence, not storage/final-candidate qualification.

### Run journal ambiguity after durable reopen — 2026-09-09

Added real session-store close/reopen tests for duplicate outcome status and verification flags. Each checks malformed payload actually exists in stored JSONL, repeated Controller reads expose no run list, rejected new runs cannot append, and file bytes remain unchanged after close. Twenty race-enabled repetitions passed 60 records, zero failures/skips. Evidence: `run-journal-reopen-integrity.json`. Clean reopen only, not crash/power-loss qualification. No production change or acceptance closure.

### Corrupt reopened journal blocks application execution — 2026-09-09

Extended both disk-backed ambiguity fixtures through two Service.Execute calls each. Twenty race-enabled repetitions passed 60 records, asserting no backend calls, zero iterations, infrastructure failure, exactly one disclosed terminal error, no text/tool events, released ownership and unchanged file bytes. Evidence: `run-corrupt-admission.json`. Service-level probe with actual storage, not built CLI/crash qualification. No production change or final acceptance closure.

### Fallback accounting through Hand and durable session — 2026-09-09

Added actual Harness fallback execution with deterministic primary overload and successful cached-input fallback. Twenty race-enabled repetitions passed, asserting primary/fallback identities, unknown failed attempt, retry provenance, 40 input/5 output with cache subset 20, incomplete wire summary, persisted reopen totals and idempotent observation replay. Evidence: `fallback-usage-persistence.json`. No live calls or production changes; tariff/budget and final acceptance remain separate.

### Fallback retains token and cost reservations — 2026-09-09

Extended actual fallback execution with session token ceiling and strict fixed route-specific test tariffs. Twenty race-enabled repetitions passed: unknown primary keeps its full positive token reservation; successful fallback charges 45 tokens without adding cached input again; cost commitment is 200 nano-units with one uncertain attempt. Both Controller budget views survive disk close/reopen unchanged. Existing usage and wire oracles still pass. Evidence: `fallback-budget-accounting.json`. Artificial deterministic tariffs, no paid calls; no production change or acceptance closure.

### Strict fallback cost refusal before provider dispatch — 2026-09-09

Added actual fallback control-flow cases for missing/expired fallback tariff and primary reservation consuming cost ceiling. Twenty race-enabled repetitions passed 80 records; provider receives primary only, error is emitted, usage has one unknown attempt, and strict budget retains the first 100 nano-unit reservation without unpriced admission. Evidence: `fallback-cost-refusal.json`. Deterministic provider/artificial tariffs, no paid calls or production change; final acceptance open.

### Full run/fallback refresh and user-approved coverage revision — 2026-09-09

Full native race suite passed 1,802 records across 26 packages, no failures/skips, unchanged Hand/Harness sources, in 179.77 seconds. Overall 81.8998%, changed 81.6351%, all critical groups above 80%. Evidence: `run-fallback-native-suite.json`. User explicitly accepted 80% coverage; plan, definition of done and checker now require 80% overall/changed/per-critical-group. Checker boundary suite passed (80 accepted, below 80 rejected). Evidence: `coverage-policy-revision.json`. Historical 90% statements remain historical. Six missing platform profiles and final candidate/Harness/platform/live/review gates remain open. Budget audit incorporates fallback integrations; no final completion claimed.

### Accounting semantic audit and display corrections — 2026-09-09

M1.1 audit found TUI still added cache subsets and used cumulative turn tokens for context. Corrected canonical totals, added context_usage runtime/application/wire events and separate request gauge state; missing usage is unknown, model/session/compaction transitions clear it. New ten-request regression failed before fix; all 326 TUI records and selected native app usage/context tests passed afterward. Initial sandbox listener failure preserved. Evidence: `request-context-accounting.json`; five-scenario mapping: `accounting-scenario-audit.json`. Full coverage stale after production changes; no final qualification claimed.

### Ten actual runtime requests through service and TUI — 2026-09-09

Added full in-process Harness/service/TUI chain, with real disk session, ten provider calls and nine tools. Twenty race-enabled repetitions passed 60 records across known/unknown final usage variants. Known final context is 20k of 100k with 201000 cumulative tokens; missing final usage displays unknown context and subtotal 180900 with one unknown attempt. Both persist across close/reopen unchanged. Evidence: `request-context-integration.json`; accounting scenario audit updated. No production change; built terminal/live/final-candidate gates remain separate.

### Full native per-request context refresh — 2026-09-09

Uncached native race/coverage suite passed 1,807 records across 26 packages, zero failures/skips, unchanged source inventories, in 176.84 seconds. Overall 81.8837%, changed 81.6324%; all critical groups above revised 80% minimum. Six platform-only profile files remain missing. Evidence: `context-native-suite.json`; current reports refreshed. Build/vet and 574-file formatting checks passed; build emitted a denied module-stat-cache write warning despite exit zero, disclosed in `context-static-checks.json`. Final clean/released/platform/live/review gates remain open.

### Search scenario audit and partial-result oracle — 2026-09-09

Mapped all eight M1.2 scenarios to inspected tests and implementation. Strengthened unreadable-file regression to require retained readable match alongside incomplete/issue metadata. Twenty race-enabled search repetitions passed 360 records, zero failures/skips. Evidence: `search-scenario-tests.json`; mapping: `search-scenario-audit.json`. Pre-cancel fixture does not establish active-scan latency; final platform/candidate gates remain explicit. No production change or final qualification.

### Active-scan cancellation preserves partial search results — 2026-09-09

Added deterministic context checkpoint cancellation after matching has begun. Twenty race-enabled repetitions passed, requiring nonempty partial prefix, explicit cancelled/incomplete state without truncation or no-match misclassification, and a successful subsequent fresh-context search. Evidence: `search-active-cancellation.json`; search audit updated. This is cooperative scan cancellation, not UI-delivery latency or stalled filesystem cancellation. No production change or final qualification.

### Permission scenario audit and runtime skill mutation denial — 2026-09-09

Mapped all five M1.3 scenarios. Added real runtime/tool/store denial cases for create/patch/replace/remove in both approval modes. Twenty race-enabled repetitions passed 180 records, zero failures/skips; existing file bytes and absence of new skill preserved, valid-input positive control changes target afterward. Evidence: `skill-permission-runtime.json`; mapping: `permission-scenario-audit.json`. Initial wrong event constant compile failure preserved. Deterministic interactive sender and direct permitted control are not built-client approval evidence. No production change/final qualification.

### Built CLI skill mutation approval journey — 2026-09-09

Added compiled CLI test for four valid mutations, each invoked without approval then with explicit --yes in a fresh process. Eight child invocations passed; local fake provider receives scoped denial, protected files stay unchanged, and approved create/patch/replace/remove effects are verified on disk. Five test/subtest records passed. Evidence: `skill-permission-binary.json`. Native macOS ordinary child build under race-enabled test; actual interactive terminal and final platform/candidate evidence remain open. No paid calls or production change.

### Scoped skill mutation through TUI decision handling — 2026-09-09

Added actual Harness skill creation blocked on application-owned scoped approval, with TUI Update y/n/Ctrl-C decisions. Twenty race-enabled repetitions passed 80 records: file absent before decision, created only for y, pending state/ownership released for all decisions and correct completed/cancelled terminal result. Evidence: `skill-tui-approval.json`; permission audit updated. In-process input routing, not built PTY rendering or persistent grant journey. No production change/final qualification.

### Real built-terminal scoped skill decisions — 2026-09-09

Added check_skill_approval_pty.py, built Hand and exercised actual PTY submission, approval rendering and y/n/Ctrl-C delivery against local fake provider. All three cases passed with unchanged Hand/Harness inventories: no write before decision, exact skill body only on y, rendered terminal outcome and exit zero. Initial allow run sent exit before terminal completion and timed out; preserved raw logs, corrected runner synchronisation and fresh run passed. Evidence: `skill-approval-pty.json`. Native development checkout; released-module provenance support and final platform/candidate repetition remain open. No production change or paid calls.

### Terminal runner provenance and failed-run reporting — 2026-09-09

Added non-Git dependency inventory support and durable failure reports for module lookup/build failures. Fresh built native PTY run passed deny/allow/cancel with unchanged Hand/Harness inventories and all exits zero. Two runner unittest methods (four subcases) verify failed setup/build reports, non-Git inventory routing and refusal after mid-run dependency mutation; two shared inventory tests also passed. Evidence: `skill-pty-runner-provenance.json`. Actual PTY used local Harness; released-module routing checks are tooling fixtures, not product qualification. No production changes or paid calls; final released candidate and platform evidence remain required.

### Built CLI signal shutdown and lifecycle audit — 2026-09-09

Added TestBinarySignalsCancelStreamingRequest: actual compiled CLI receives SIGINT/SIGTERM during a streamed local-provider request. Twenty repetitions per signal passed, 40 child journeys and 60 test records: exit 130, request connection closed, ordered JSONL with exactly one final Cancelled outcome and no later events. Related stale-result, compaction, outcome matrix and mandatory-hook selection passed 740 records across twenty repetitions with no failures/skips. M1.4 now maps all eight scenarios in `lifecycle-scenario-audit.json`, with explicit remaining built-client, session-new, mandatory tool timeout and final platform/candidate gaps. No production changes or paid calls.

### Mandatory tool timeout and queued goal after /new — 2026-09-09

Built CLI mandatory PreToolUse timeout test passed once then twenty consecutive race-parent repetitions (40 repeated child invocations). Explicit --yes cannot bypass timed-out validator; target stays absent and provider receives denial. Successful validator then permits the same mutation. Added actual TUI /new with session store and queued completed goal result delivered both before/after session selection: twenty race repetitions passed with no new work or history contamination. Evidence: `lifecycle-gap-tests.json`; lifecycle audit updated. No production changes; built PTY session-new, supported platforms and clean released-candidate qualification remain open.

### Compiled CLI exit-code matrix — 2026-09-09

Added nine fresh-process cases covering answer/verified/optional warning, Stop and prompt validation failures, iteration/turn limits, provider rejection and invalid invocation. Ten test records passed: exact exit codes 0/2/3/4/5, expected status/reason/verified fields, request counts and exactly one ordered final JSONL outcome for started runs. Invalid input makes no provider call or runtime terminal event. Signal 130 remains covered by the separate 40-journey suite. Evidence: `outcome-binary.json`; lifecycle scenario audit updated. No production changes or paid calls; final native-platform/released-candidate qualification remains open.

### Profile metadata failure/recovery and M2.2 audit — 2026-09-09

Added real HTTP metadata failure followed by successful TUI profile switch: twenty race repetitions preserve old model/context/gauge/compaction on failure, then apply active 24576 context (not advertised 262144) and reset gauge on success. Broader profile selection passed 440 records across twenty repetitions with no failures/skips. Mapped all eight M2.2 scenarios in `profile-scenario-audit.json`, retaining explicit catalogue, actual HTTP switching, post-switch summarization and final candidate/platform gaps. Corrected stale metadata-startup wording in model-profiles.md. No production changes or paid calls.

### Configurable generation output limits — 2026-09-09

Implemented prerequisite for catalogue resolution: Harness Runtime/AgentSpec MaxOutputTokens propagated by Run and RunTurn including fallback; zero retains 8192 and negatives reject. Hand applies profile max_output at startup and atomic switches, resetting to default when omitted. Actual local HTTP request verifies 123 through max_completion_tokens. Initial assertion read wrong wire field; preserved failure and corrected from adapter source. Affected profile tests and full Harness runtime suite passed; evidence `output-limits.json`. Previous aggregate coverage marked stale. Harness release, catalogue wiring, fresh Hand suites and final platform/candidate evidence remain pending. No paid calls.

### Versioned catalogue wired into profile resolution — 2026-09-09

Added documented GPT-4o exact-ID catalogue version 2026-09-09.1 and connected resolver to startup/profile switching. Explicit/server metadata precede catalogue; unknown/custom/local routes use labelled 8192-context/2048-output fallback rather than family guesses. Input modalities clone and explicit declarations win. Complete config/app/TUI run: 746 passes, zero failures, five missing-fixture skips retained as pending. Focused CLI/catalogue run passed. Initial stale proxy-output assertion retained and corrected to resolved fallback. Evidence: `catalogue-integration.json`; catalogue source/maintenance and behavioural migration documented. Full coverage remains stale; released Harness, platform fixtures and final qualification still pending. No paid calls.

### Full native suites after output/catalogue integration — 2026-09-09

Fresh uncached race/coverage runs with container/public-fetch fixtures passed without skips or failures and unchanged before/after inventories: Hand 1843 records/26 packages in 188.23s; Harness 1112 records/32 packages in 134.30s. Hand statement coverage 81.93164%; changed observed coverage 81.68223%; every critical group above authorised 80% threshold. Six platform-specific files still lack profile coverage, so changed coverage is not finally qualified. Harness total 70.02019% is reported separately; combined critical groups meet current numeric minimum. go vet ./... passed. Evidence: catalogue-native-suite.json, catalogue-harness-suite.json, catalogue-static-checks.json. Current native metrics refreshed; dirty development checkouts, released dependency, other platforms and final candidate gates remain open.

### Isolated non-root Linux checkpoint suite — 2026-09-09

Added check_checkpoint_linux.py and ran complete checkpoint package in Linux arm64 container as host non-root UID/GID with network disabled, read-only root and no Docker socket mount. Native tmpfs exercises atomic rename/exchange and process recovery. All 93 records passed, no skips/failures, source inventory unchanged; package coverage 82.09693372898121%, rename_linux.go 100.0%. Evidence: `checkpoint-linux.json`. Cross-compiled non-race package run is supplementary; full Linux container-boundary suite remains blocked on separately required access, final other-platform coverage and candidate qualification remain pending.

### Unsupported-platform coverage scope proposal — 2026-09-09

Fresh Go build inventories for all four advertised Linux/macOS targets prove five missing-profile _other.go files are excluded on every supported target. Current contract permits only tests/generated exclusions, so no policy was changed. Prepared fingerprinted platform-coverage-scope-proposal.md/json for explicit user decision. Linux rename remains required and separately measured; all thresholds/scenarios/platform gates unchanged.

### Real HTTP clients after profile switches and summary — 2026-09-09

Twenty race repetitions passed local → hosted-adapter → proxy → local with actual provider construction: 80 profile switches and 160 generation/summary HTTP requests. Endpoint, exact model, scoped dummy Authorization and output limits (generation 123, summary 512) asserted per request; return to local clears credentials despite ambient keys. Evidence `profile-http-routing.json`; M2.2 route/credential/compaction audit updated. Local fixture endpoints, not paid hosted evidence or compiled terminal journey. No production changes. Coverage scope proposal still awaits user decision; current policy unchanged.

### Interactive profile picker — 2026-09-09

Implemented bounded keyboard selection and detached model/configuration preview; Enter reuses asynchronous profile switching and supports exact names containing spaces. Escape/Ctrl-C preserves draft and model; opening picker invalidates queued goal continuation. Final full TUI suite passed 336 records, no failures/skips. Evidence `profile-picker.json`. Full coverage marked stale after production change; actual built PTY selector journey remains pending. User asked whether swarms are in scope: current plan/requirements contain no explicit swarm or multi-agent orchestration deliverables; no scope added.

### Built terminal profile picker journeys — 2026-09-09

New check_profile_picker_pty.py uses shared source/build provenance runner. Real 80x24 PTY tests pass selection, Escape dismissal and Ctrl-C dismissal. Picker opening makes no model request. Subsequent actual HTTP request proves selected model/output (selected/512) or retained model/output (original/123), no auth, one terminal Completed result and clean process exit in each case. Hand/Harness inventories unchanged during run; shared runner failure/provenance tests pass after callback generalisation. Evidence `profile-picker-pty.json`. Native development evidence; final platform/released-candidate and live evaluation gates remain open.

### Shared service ownership audit — 2026-09-09

Sixteen service tests passed twenty race repetitions: 320 top-level passes, 380 including subtests, zero failures/skips. Main CLI/TUI use Service.Start, but Runner-only TUI construction still owns a legacy turn/goal loop; main also performs startup runtime configuration. M2.1 remains incomplete. `service-ownership-scenario-audit.json` maps all five scenarios and records constructor/test migration and legacy removal, with archived source and raw output. No product change, aggregate coverage refresh or final candidate qualification claimed.

### Service-required terminal construction — 2026-09-09

Main now uses NewApplicationModel(controller.Owner), rejects missing service and installs no legacy Runner or TUI Stop-hook configuration. Continuation display derives iteration from the service event. Shared two-iteration verification regression passes without legacy setup; full TUI race suite passes 337 records, focused app/compiled CLI matrix 28. Compiled PTY profile selection/Escape/Ctrl-C pass with unchanged Hand/Harness inventories. Initial sandbox listener failure is archived alongside authorised success. Evidence: `application-constructor.json`. Legacy constructors/tests and loop removal remain; no full M2.1 or final coverage qualification claimed.

### Fixed terminal service ownership — 2026-09-09

Removed SetService and migrated all 13 service-backed fixtures through the production constructor. Runner-accepting NewModel now exists only in test code; production presentation initialization is private. Full terminal race suite passed 337 records, no failures/skips; CLI build passed (non-fatal sandbox module-stat-cache warning recorded). Evidence: `service-setter-removal.json`. Legacy event/goal handlers and Runner-only fixture migration remain; aggregate coverage remains stale and M2.1 incomplete.

### Outcome fixtures use application service — 2026-09-09

Migrated all five outcome tests (including eight-case matrix) from legacy TUI runtime/goal messages to the production constructor and service events. Preserved real Stop validators, final-iteration verification, duplicate-terminal suppression, cancellation after successful validation before commit, and interactive one/two-turn results. Twenty race repetitions passed 260 records with no failures/skips. Evidence: `outcome-service-migration.json`; lifecycle mappings refreshed for changed test source. Test-only change; remaining legacy fixture migration and handler removal still required.

### Nine goal-loop regressions migrated — 2026-09-09

Moved nine existing regressions from model_test.go to service_goal_test.go, retaining names for traceability and replacing legacy internal message injection with complete application-service journeys. Real Stop subprocesses verify stdout continuation, final-iteration checks, two-turn cap and terminal verification; cancellation/error prevent checks and continuation. Source transcript assertion distinguishes automatic continuation from user input. Twenty race repetitions passed 180 records, zero failures/skips. Initial compile error and corrected output retained in `service-goal-migration.json`. Legacy stale-event/lifecycle fixtures and production handler removal remain; no final qualification claimed.

### Service cleanup and stale cancellation regressions — 2026-09-09

Migrated three lifecycle tests to service-owned streams. Done-before-channel-close rejects overlapping work and withholds terminal; Ctrl-C cancels checker context while retaining ownership until join; late continuation/success from cancelled stream cannot change outcome, transcript or backend count. Twenty race repetitions passed 60 records, zero failures/skips. Removed now-unused SetGoalLoop production method. Evidence `service-cleanup-migration.json`; affected lifecycle mappings refreshed. Other legacy stale-session/model fixtures and duplicate handlers remain; full qualification not claimed.

### Service continuation across session change and duplicate delivery — 2026-09-09

Migrated new-session and duplicate-continuation regressions. Actual Stop-generated service event is replayed during and after /new; old work cannot start and fresh history remains empty. Duplicate active continuation while second turn is held starts no third backend turn, then completes verified at two iterations. Twenty race repetitions passed 40 records with no failures/skips. Evidence `service-session-migration.json`; affected lifecycle mapping refreshed. Test-only migration, not presentation deduplication or final platform/candidate qualification.

### Duplicate terminal goal loop removed — 2026-09-09

Migrated the remaining goal-check message tests (140 passing records over twenty race repetitions), then removed goalLoopResultMsg/handler, maybeContinueGoalLoop, startAutoContinue and hook configuration fields. No production TUI calls EvaluateStopHooks. Full terminal race suite passed 337 records with no failures/skips; CLI build passed. Evidence `legacy-goal-removal.json`. Legacy single-turn runtime/event/approval adapter and fixtures remain; startup configuration extraction and final qualification are still pending.

### Rendering regressions use application events — 2026-09-09

Migrated 10 rendering regressions to production construction/application events across model, Markdown and output tests. Retained terminal sanitisation, live/replay output equality, selected Markdown style, tool counters and usage totals; usage checks compare copied values instead of mutable pointer identity. Full terminal race suite passed 337 records with no failures/skips. Evidence `service-rendering-migration.json`. Test-only change; runtime/error/approval fixture migration and legacy adapter removal remain.

### Interactive stream and error rendering migration — 2026-09-09

Migrated three Bubble Tea text/tool journeys and the raw-error sanitisation regression to service-backed execution. Twenty race repetitions passed 80 records without failures/skips. No remaining test directly calls handleAgentEvent; indirect legacy runtime/approval adapters still have callers. Evidence `service-stream-rendering.json`. Test-only migration; final candidate/platform coverage remains unqualified.

### Terminal approval fixtures use service broker — 2026-09-09

Migrated four interactive approval regressions to real application broker requests instead of injected response channels. Yes/no/Enter/always and actual command preview remain covered; store reopen additionally proves always persists while once/deny do not. Twenty race repetitions passed 80 records with zero failures/skips. Initial unused-import build failure retained in `service-approval-terminal.json`. Uses legacy permission-store policy behind service broker; scoped authority and tool execution have separate evidence. Remaining legacy identity/image/runtime fixtures and production adapter removal are still pending.

### Image input fixtures use service backend — 2026-09-09

Migrated both image-path regressions to service-backed execution. Assert exact attachment bytes/MIME, one backend invocation with original filename, transcript placeholder, and zero backend calls for unapproved outside-workspace paths. Initial assumptions about model placeholder/outside invocation failed; corrected against parser contract and retained output. Twenty race repetitions passed 40 records with no failures/skips. Evidence `service-image-input.json`; no decoder/live vision or final candidate claim.

### Identity/cancellation/quit fixtures use live service approvals — 2026-09-09

Migrated stale identity, cancellation/late output and guarded quit regressions using an actual pending service approval with backend cleanup held on a channel. Full identity mismatches cannot resolve the request; cancellation and quit retain ownership until backend join, discard late text and reject duplicate terminal delivery. Twenty race repetitions passed 120 records including subtests, no failures/skips. Evidence `service-identity-migration.json`. Test-only migration; legacy adapter removal remains.

### Terminal runtime adapter removed — 2026-09-09

Removed Runner field/interface, raw runtime-event/closure handlers, renderer, completeTurn and StreamEvents forwarding. Migrated final streaming/order/duration/compaction/session fixtures; presentation fixtures now use service construction. Full terminal race suite passed 337 records, no failures/skips, after correcting archived leftover resets/import errors. Actual compiled PTY selection/Escape/Ctrl-C all passed with unchanged source inventories. Evidence `runtime-adapter-removal.json`. Transitional Controller.Run, channel approval compatibility and main runtime construction remain; M2.1 and final aggregate coverage not qualified.

### Controller runtime adapter removed — 2026-09-09

Removed transitional Controller.Run after migrating ownership and run-budget tests to service Start/Wait. Twenty race repetitions passed 120 records with no failures/skips: configuration stays blocked until cancellation joins, token/time budget causes survive service outcomes, and manual compaction accounting remains governed. CLI build passed. Evidence `controller-adapter-removal.json`. Startup runtime configuration extraction, legacy approval-message compatibility and final candidate/platform qualification remain.

### Initial runtime configuration owned by app — 2026-09-09

Main no longer assigns runtime fields directly; app.ConfigureInitialRuntime validates limits/reasoning before applying resolved settings. Atomic-rejection/default tests passed (2 records); CLI named-profile request and compiled outcome matrix passed 11 records, no failures/skips. Evidence `initial-runtime-config.json`. This extracts startup field mutation; CLI still composes runtime construction and startup presentation. Legacy approval compatibility and full final candidate/platform qualification remain.

### Legacy terminal approval transport removed — 2026-09-09

Final identity/viewer/scrolling fixtures use live service approvals (60 passes over twenty race repetitions). Removed raw approval request/warning handlers and Respond-channel fallback. Full terminal race suite passed 337 records, no failures/skips. Compiled PTY deny/allow/cancel journeys passed with unchanged source inventories; actual skill file created only when allowed. Evidence `approval-compatibility-removal.json`, including initial unused-import build failure. Architectural review, fresh aggregate coverage and final released-candidate/platform qualification remain; M2.1 not marked complete.

### Full native lifecycle refresh — 2026-09-09

Corrected full native uncached race/coverage suite passed 1848 records across 26 packages, no failures/skips, in 174.26s with unchanged source inventories. Overall statement coverage 81.95697%; observed changed coverage 81.72932%; all nine critical groups exceed 80%. Harness source inventory equals its prior full-suite inventory, allowing that coverage profile to be reused. Initial run failed because rebuilt extension peer targeted internal/extensions instead of internal/app; retained raw failures and fixed permanent Linux runner with entry-point validation. Evidence `lifecycle-fixed-native-suite.json` and `lifecycle-refresh-fixtures.json`. Changed coverage still reports six platform files plus now type-only internal/tui/events.go as missing profiles; investigate type-only accounting before qualification. Supported platforms, released Harness and clean final candidate remain open.

### Declaration-only coverage accounting — 2026-09-09

Go’s instrumenter confirms internal/tui/events.go has zero coverage counters after legacy adapter removal. Reporter records the proof without changing covered/total statements or the 80% threshold. Nine reporter tests and six validation-runner tests pass, including executable initializers, platform-tagged functions, spoofed metadata and symlink rejection. Recomputed the authentic native profile after verifying unchanged Go/module inputs: observed changed coverage remains 81.72932%, with six executable platform files still missing. Historical suite report retained. Evidence `zero-counter-accounting.json`; final candidate and platform qualification remain open.

### Startup attempt ownership extracted — 2026-09-09

Application StartupAttempt now owns construction cancellation, join, failed runtime cleanup and transfer. CLI retains draft/status rendering and rejects stale attempt notifications. Missing builder/results fail closed; late successful builders cannot transfer after cancellation. Six regression tests over twenty race repetitions passed 120 records, no failures/skips. Evidence `startup-owner.json`. Aggregate profiles marked stale after production changes; real MCP cleanup integration, remaining CLI dependency composition and final candidate qualification remain.

### Startup ownership with real MCP child — 2026-09-09

Twenty race repetitions of four actual MCP stdio journeys passed: transfer, terminal disconnect, late cancellation after connection and partial startup failure. 80 journeys / 100 test records, zero failures/skips. Successful transfer preserves the connection until owner Close; all abandonment paths transfer no runtime, retain error causes and join the child before returning, verified by its shutdown marker. Evidence `startup-mcp.json`. Mid-handshake cancellation and final candidate/platform qualification are outside this focused test; no aggregate coverage refresh claimed.

### Cancellation during real concurrent MCP startup — 2026-09-09

New regression waits for two actual MCP child PIDs, cancels while one handshake is stalled, and requires both PIDs reaped, no runtime transfer, no partial catalogue, context.Canceled and cleanup within two seconds. Initial test incorrectly assumed sequential connections and required graceful marker from the other child; corrected against Harness concurrent startup using process-liveness assertions, preserving failed logs. Combined handshake and connected-runtime suite passed 120 records over twenty race repetitions, zero failures/skips. Evidence `startup-handshake.json`; final candidate/platform qualification remains open.

### Compiled coding/edit/test/resume journey — 2026-09-09

Added integrated CLI regression: original addition fixture fails its independent Go test, explicit --yes invocation writes correction and runs actual go test through bash, then a fresh process resumes the same session with both tool results and preserves an intervening user edit. One uncached race-enabled harness run passed; child binary uses ordinary go build. Sandbox loopback bind failed; approved native local HTTP execution passed and both outputs retained. Evidence `coding-journey.json`. G-JOURNEY remains pending final candidate and full scenario qualification; no live model-quality claim.

### Same coding journey through public SDK and compiled RPC — 2026-09-09

Refactored compiled coding journey to run both CLI and sdk.StartProcess against identical edit/test/resume assertions. SDK negotiates, submits prompt, answers exactly two run-bound one-time approvals, checks completed terminal, joins child and restarts the same persisted session. Both uncached journey tests pass. Initial test used invalid approval value allow; protocol correctly rejected it, corrected to once and retained output. Evidence `coding-rpc-journey.json`; final candidate and live task quality remain unqualified.

### Full native startup and coding-journey refresh — 2026-09-09

Uncached full Hand race/coverage suite passed 1858 test records across 26 packages with zero failures/skips in 193.61 seconds. Rebuilt Linux worker/extension-peer fixtures first; Hand and Harness before/after inventories unchanged. Overall coverage 81.99357%, observed changed statements 81.76752%; all nine observed critical groups exceed 80%. Harness inventory matches its previous full-suite profile. Declaration-only events.go remains proven zero-counter; six executable platform files remain missing from native changed coverage. Evidence `startup-journey-native-suite.json`; dirty checkout, local unreleased Harness, remaining platforms and final acceptance still unqualified.

### Compiled RPC restore with intervening user edits — 2026-09-09

Extended coding journey with actual checkpoints across process restart. Uses checkpoint.changes identity, rejects stale preview through completed/error result, observes fresh conflict and unchanged user code, then explicitly returns selected file to post-image and restores exact starting content while preserving unrelated edits/artifacts. One uncached native journey passes. Two initial test API assumptions corrected with failed logs retained; evidence `restore-journey.json`. Final candidate/scenario qualification remains pending.

### Compiled SDK steering during tool and cancellation — 2026-09-09

Actual Hand subprocess executes an approved bash child, accepts and exposes queued steering while the child is alive, then cancels. Terminal is cancelled within two seconds after the child is reaped; orderly shutdown and exactly one provider request prove no queued dispatch after cancellation. Initial uncached run plus required twenty race-enabled repetitions pass without failures/skips. Evidence `steer-cancel-journey.json`; final candidate/platform qualification remains pending.

### Seven-journey audit and static validation — 2026-09-09

Mapped all seven G-JOURNEY scenarios without qualifying any final scenario: four have recent compiled development tests, extension reload has an existing runner with historical pre-integration evidence requiring refresh, and compiled crash/budget and isolated background-server journeys remain missing. Fixed go vet lock-copy warning in atomic configuration test using independent expected Runtime; full go vet and two focused configuration regressions pass. Evidence `journey-scenario-audit.json`, with initial vet failure retained.

### Current compiled Go/Python extension reload journeys — 2026-09-09

Rebuilt Hand and Go peer, then executed existing RPC extension runner for Go/Python plus twenty repetitions (40 repeated language journeys), all passed with unchanged source inventories. Covers reviewed activation, question/answer and durable replay, reuse, failed staged replacement retaining old peer, cancellation recovery, remove-all and snapshot cleanup. Archived raw runs and fingerprints in `extension-journey-refresh.json`. Journey audit now records five recent development journeys; crash/budget and isolated background-server compiled journeys remain missing, all final qualification remains pending.

### Compiled crash/restart with retained token budget — 2026-09-09

Actual Hand SIGKILL during provider stream leaves uncertain RPC request and positive reserved token charge. Two subsequent process starts preserve charges; insufficient headroom blocks provider dispatch, explicit increase permits completed resumption without erasing uncertain usage. Twenty race-enabled crash journeys pass, zero failures/skips. Initial test ceiling-equality rejection retained and corrected to committed+1. Evidence `crash-budget-journey.json`; isolated background-server journey remains missing, all final candidate/platform qualification remains pending.

### Compiled RPC isolated background HTTP server — 2026-09-09

Permanent runner launches approved background Python HTTP server via actual Hand process tool and container backend. Server serves expected bytes inside network-none boundary; Docker inspection proves read-only root/workspace/worker mounts, no outside mounts; in-container probe cannot see outside sentinel or Docker socket. Hand shutdown removes owned container. Initial readiness and worker-mount test assumptions corrected with failed evidence retained. Evidence `background-server-journey.json`. All seven G-JOURNEY scenarios now have development executions; none is final candidate/platform qualified.

### Current startup and transcript performance — 2026-09-09

Sequential measurements avoid benchmark interference. 120 PTY startup trials: no-MCP p95 92.19ms vs baseline-derived 116.61ms threshold, with identical hardware/platform/toolchain/terminal fields; slow optional MCP p95 95.08ms. Transcript: 10,000 blocks, 30 runs x 1000 events, reported p95 0.449792ms and worst-run p95 0.47575ms, scope excludes terminal display/OS events. Initial transcript hardware sysctl was sandbox-blocked; approved native measurement passed. Raw evidence `performance-refresh.json`. Other performance scenarios and final candidate qualification remain open.

### Cancellation timing refresh including startup — 2026-09-09

Added production startup UI Ctrl-C/Escape/disconnect timing regression. Refreshed backend/checker, viewer/output loading, compaction, session operation and startup paths: 22 paths x 30 trials = 660 trials, all pass; maximum path p95 1.496042ms vs 1000ms limit. Raw samples/source/tests archived in `cancellation-performance-refresh.json`. Context delivery scope excludes OS/terminal and remote provider latency; approval/extension/verification inventory reconciliation and final candidate/platform qualification remain open.

### Pending approval cancellation timing and join boundary — 2026-09-09

Added five real service-broker UI cancellation paths, thirty trials each (150 total): Ctrl-C, quit, close, viewer and viewer search. Cancellation unblocks approval with cancelled context, terminal remains withheld while backend cleanup is held, then joins with cancelled outcome. All race-enabled trials pass; max path p95 0.026458ms. Evidence `approval-cancellation-performance.json`; extension/verification timing inventory and final qualification remain open.

### Verification cancellation with actual child cleanup — 2026-09-09

Added confirmed verification-process cancellation across Ctrl-C, quit, exit and close, thirty trials each. All 120 trials pass under race detector: child PID reaped before operation result, guard released, no passed label after cancellation. Maximum complete cleanup 19.037625ms vs 5000ms deadline. Evidence `verification-cancellation-performance.json`; scope is UI dispatcher to complete worker result, not PTY/OS input latency. Final candidate/platform qualification remains pending.

### Integrated output flood and retention caps — 2026-09-09

Hand process manager drains actual 20MiB stdout + 20MiB stderr, retaining exact 64KiB prefixes and 8MiB private artifacts per stream. Hand admission/retention tests and Harness artifact quota, cross-process reservations and 24h expiry eviction tests pass under race detector. Evidence `output-caps.json` records 64MiB store quota, 8 active/32 retained handles and retained-payload scope. Total heap/RSS and allocator/buffer overhead are not established by these assertions; final memory accounting and candidate/platform qualification remain open.

### Harness flood allocation fix — 2026-09-09

Memory audit regression failed: saturated OutputCapture.Write copied the 64KiB prefix once per write just to read accounting. Added allocation-free Capture.stats and limited prefix copying to the initial spill boundary. Regression now shows zero allocations per saturated write and 81920-byte backing capacity within 139264-byte allowance for 65536-byte prefix. Focused Harness capture/artifact/handle race tests and integrated Hand 40MiB flood pass. Evidence `capture-memory.json`, including failing baseline and source snapshots. Aggregate profiles marked stale after dependency production change; Harness release and full candidate qualification remain open.

### Critical coverage zero-counter accounting — 2026-09-09

Extended the instrumenter-backed declaration-only proof to exact critical-group source paths. Removed the false missing-profile gap for internal/tui/events.go without changing any covered/total statement count or measured percentage. Wildcards, executable platform sources, unsafe paths, absent modules and pending capabilities remain incomplete. Seven reporter and six validation-runner regressions pass. Evidence: `critical-zero-counter-accounting.json`; historical profiles remain stale after the Harness capture fix. Full Harness refresh was rejected before execution by automatic approval review reporting an account usage limit; no test process started and no substitute full-suite result was recorded.

### Completion checker dependency and evidence boundaries — 2026-09-09

Audited actual final checker and closed two enforcement gaps: evidence must resolve outside the checkout, and effective Harness must match the go.mod pin without a workspace-main module or replacement. Go module inspection disables proxy/checksum/toolchain downloads. Current checkout correctly reports not_complete and detects the local workspace Harness. Full runner suite 122 tests passed; checker suite 20 passed after the fix. Evidence `acceptance-identity.json` archives source hashes and raw outputs. Synthetic checker tests are tooling evidence only; no product scenario or release gate was closed. Full Harness refresh remains blocked before execution by automatic approval service usage limit.

### Refreshed reconstructable Harness release handoff — 2026-09-09

Rebuilt cumulative patch from historical base in temporary checkouts; all 300 files, symlinks and executable modes match current integrated Harness, including capture allocation regression. Original source/index unchanged; reconstructed `go vet ./...` passes with workspace/proxy disabled. Evidence `harness-capture-review.json`. Updated current release pointers and marked obsolete proposal historical; no frozen candidate, release approval, publication or fresh full-suite qualification is claimed. Full race/coverage execution remains blocked before start by the approval service usage limit.

### Application-owned runtime construction — 2026-09-09

Moved required/optional MCP startup selection and production Harness constructor calls from main/startup UI into app.RuntimeConstruction. Configuration snapshots protect argument/env/header collections; duplicate/empty names are rejected consistently; caller cancellation is honoured on the no-server path. Three app tests and eight startup test/subtest records pass under race detector, including actual MCP child ownership/close. Evidence `runtime-construction.json`. Full native/profile freshness remains false; final release/platform acceptance is pending.

### Removed legacy runtime constructor — 2026-09-09

Moved all four existing timeout/MCP regression tests to the application constructor and removed unused agentio constructor/constant. Startup deadline now belongs to app. Per-test real MCP binaries are cleaned by t.TempDir instead of being left for OS cleanup. Strengthened timeout assertion to require context.DeadlineExceeded and preserved partial-catalogue/connected-child cleanup checks. Seven focused race tests pass, including 15-second real partial-connection timeout; vet passes in all three affected packages. Critical mapping follows moved implementation with no exclusions. Evidence `runtime-timeout-migration.json`; full candidate/platform qualification remains pending.

### Signal shutdown during MCP startup — 2026-09-09

New actual-binary regression demonstrated SIGINT/SIGTERM terminated Hand and left its MCP child alive in both one-shot and RPC startup. Carried one signal context through profile/backend preparation, session/runtime construction and execution; cancellation before configuration readiness is no longer misclassified as an invocation error. RPC inherits the same context. Initial four-case regression now passes; 20 repeats produce 80 startup signal trials plus existing one-shot signal checks (160 passing test/subtest records). Maximum startup child cleanup 266.058ms; all children gone before exit 130 and no fabricated run output. Eleven outcome test/subtest records and vet pass. Evidence `startup-signal.json` preserves failure and passing logs. Full candidate/platform qualification remains pending.

### Real extension question cancellation timing — 2026-09-09

Added six UI paths (Ctrl+C, quit, exit, close, viewer, viewer-search), thirty trials each using reviewed compiled Go extension peers waiting for answers. Final 180 trials pass under race detector; cancellation returns context.Canceled, removes question, releases ownership, restores saved draft and leaves durable state unchanged. Maximum path p95 2.392ms; maximum joined operation 11.648125ms. Evidence `extension-cancellation-performance.json` includes both initial timing run and strengthened draft assertion run, raw samples and source snapshots. Measures dispatcher-to-operation join as conservative cancellation-initiation bound, not PTY latency or direct PID probes. Full candidate/platform qualification remains pending.

### Complete Go test inventory and terminal-event checks — 2026-09-09

Validation now inventories Test, runnable Example and Fuzz seed targets, including exact prefix names. It rejects started-but-unfinished tests/subtests and missing completion events for packages with expected tests. Missing/failing execution still preserves coverage diagnostically and cannot pass even with exit zero. Ten runner tests pass. Authentic protocol race run passes seven test/subtest records including FuzzDecodeRequest seeds; removing fuzz events from a separate copy produces a missing-target diagnostic. Evidence `runner-inventory.json` distinguishes authentic execution from tooling fault injection. Full candidate qualification remains pending.

### RPC disconnect during construction and interruptible stdio — 2026-09-09

Actual binary regression showed EOF ignored during hung MCP initialization. Started the existing bounded frame reader before construction and handed it to the dispatcher. Initial repair exposed inherited blocking stdin preventing signal joins; owned nonblocking descriptor duplicates now let Go poller interrupt reads/writes on Linux/macOS. Combined signal/EOF checks pass; twenty EOF repetitions pass with worst cleanup 258.474ms. Six transport tests x20 pass (120 records), including pre-start EOF and retained queued negotiation; actual Hand early hello/normal EOF passes. Vet passes. Evidence `rpc-startup-eof.json` preserves both failing stages. Multiple queued-frame disconnect audit, Linux/native full suites and final candidate qualification remain open; unsupported platform source is not excluded.

### Bounded RPC startup backlog and EOF — 2026-09-09

Reproduced pre-dispatcher reader blockage when two complete requests filled its queue before EOF. Startup now accepts one queued request and rejects another with explicit backlog diagnostic while cancelling construction. Ready-state Serve retains existing bounded backpressure. Compiled binary tests cover zero, one and two queued frames; all 60 trials across 20 repetitions pass, with max cleanup 263.659ms, no fabricated stdout and actual child PID gone. Seven transport regressions x20 pass (140 records); early hello negotiation and vet pass. Evidence `rpc-startup-backlog.json` preserves failing baseline. Protocol critical map now explicitly includes both stdio platform adapters; no unsupported-source waiver applied. Full platform/candidate qualification and steady-state pipeline audit remain open.

### Four-target builds after RPC stdio changes — 2026-09-09

Built development archives for darwin/linux amd64/arm64, with architecture-matched Linux workers. All archive/checksum/content/ELF-MachO/worker metadata checks pass. Native darwin-arm64 help/version smoke passes without credentials or state creation. Hand and Harness source inventories unchanged during build (34.64s) and smoke (1.74s). Recorded effective workspace Harness and Go environment; version explicitly labels dirty development state. Evidence `rpc-platform-build.json`; archive bytes in /private/tmp/hand-rpc-platform-build-20260909/dist. This is build evidence for four targets and help/version execution for one host, not native runtime/platform or final released-candidate qualification.

### RPC queued disconnect regression — 9 September 2026

Fixed the macOS peer monitor and preserved Go nonblocking descriptor registration. Twenty repetitions cover each input/output disconnect, context cancellation, and idle-read close. Final focused run: 100 passing test/subtest records, no failures or skips; startup checks: 10 passes; transport checks: 140 passes. Disabling the monitor reproduces the queued-input failure. Linux ARM64 cross-build passes, but native Linux verification and fresh full-candidate coverage remain pending. Evidence: `rpc-peer-disconnect.json`. Requirement M4.1 remains in progress.

### Preserve framing failures during RPC disconnect — 9 September 2026

Real-pipe regression exposed input hangup racing the frame decoder and hiding truncated-message errors. Input EOF now drains the finite trailing pipe data without executing queued commands; output close remains immediate. The regression asserts io.ErrUnexpectedEOF. Final combined pipe/transport run passes 260 test/subtest records over 20 repetitions. Full RPC/protocol run passes 197 records with complete observed inventory; startup checks pass 10 records. Initial truncation failures and intermediate output-close error are retained in evidence. Rebuilt all four archives and passed host macOS ARM64 smoke. Evidence `rpc-peer-framing.json`; final candidate, native Linux and full coverage remain pending.

### Python evaluation oracle support — 9 September 2026

Added an isolated-interpreter unittest driver emitting strict JSONL test events; manifest requires a hashed driver fixture and exact expected identities. Baseline/candidate verification now supports Python as well as Go. Tests reject failed subtests/cleanup, skipped and expected-failure tests, missing inventory, empty discovery and import/class setup failures; a pinned synthetic repository fails before and passes after a patch. All evaluation-tooling tests pass. This does not add synthetic tasks to the real corpus or establish live task success. Evidence `evaluation-python.json`; M8.1 remains in progress.

### First real Python comparison task — 9 September 2026

Added a pinned Kokoro-ONNX configuration-validation defect with a five-test independent oracle. Baseline: two directory-rejection failures and three compatibility passes; minimal calibration patch: all five pass, fixture integrity preserved. Expanded catalogue has four calibration tasks, three repositories, Go/Python, no held-out tasks. Fixed manifest test setup to copy every fixture rather than only the first; all 73 evaluation-tooling tests pass. Original external checkout unchanged. Evidence `kokoro-oracle.json`; real 30-task/10-held-out matrix, other categories, frozen agent settings and authorised execution remain pending.

### Pi comparison source refresh and executor contract — 9 September 2026

Identified official Pi release v0.85.1 at d981de1229ef899957bbe968bc8dcda02a21f477 and recorded package/runtime metadata with primary-source links. Runtime/binary verification remains pending. Release RPC documentation distinguishes prompt admission, low-level agent_end and full agent_settled; comparison must not stop before automatic retries/continuations finish. Documented remaining lifecycle, shared spending boundary, crash reservations, traffic isolation and fairness tests in evaluation/executor-design.md. No agent/model configuration, live execution or spend approval invented. Evidence pi-source-review.json; M8.1 stays in progress.

### Pi lifecycle observation — 9 September 2026

Implemented bounded LF-only RPC observation for one fresh Pi run with correlated prompt/statistics identities, frozen assistant model identity and full settlement. Intermediate agent_end and admission cannot finish observation; pending tools, malformed/duplicate/truncated streams and model changes fail. Retry errors remain counted, aborted/length/error terminal outcomes remain distinct, unknown costs remain null. Ten synthetic event tests and all 83 evaluation-tooling tests pass. Evidence `pi-observer.json`; actual Pi runtime, gateway enforcement and task scoring remain pending.

### Actual pinned Pi runtime — 9 September 2026

Resolved the public v0.85.1 tag to the expected commit, downloaded the source and npm package, verified package SHA-512/SHA-1 against registry metadata, and installed into a temporary prefix with lifecycle scripts disabled. Actual Node v25.8.1 runtime passes fresh-home credential-free success, automatic-retry and abort probes using a deterministic custom provider. Each process exits zero after final stats; observer accepts the actual streams. Retained those streams as permanent regression fixtures; all 84 evaluation-tooling tests pass. Evidence `pi-runtime.json`. No real-model, live-accounting, tool isolation or comparative-quality claim; package attestation/source-build correspondence and remaining evaluation gates stay open.

### Tooling refresh and explicit Docker approval gate — 9 September 2026

All 144 Python tooling tests and 20 acceptance-checker tests pass, zero failures/skips. Evidence `python-tooling-refresh.json`. Prepared a fresh current-source Harness inventory/race/coverage runner with model credentials excluded, but automatic review rejected Docker socket access as root-equivalent privilege without specific user approval; no test process started. Requested explicit approval and retained exact runner/hash/rejection in `harness-refresh-approval.json`. No bypass attempted. Full source/platform runtime gates remain pending; independent tooling results do not substitute for them.

### Durable evaluation reservations — 9 September 2026

Implemented SQLite manifest-bound budgets with transactional pre-admission request/token/cost reservations, expiry/revocation, retained uncertain charges, idempotent exact settlement and persistent violation records that block further calls. Eight focused tests and all 92 evaluation-tooling tests pass. Concurrent global-budget admission, process crash after commit and inconsistent-usage blocking each pass 20 repetitions (60 trials). Evidence `evaluation-budget-ledger.json`. This is not paid authorisation, request pricing or gateway enforcement. Docker approval remains pending; no privileged suite was started or bypassed.

### Evaluation ledger recovery fixes — 9 September 2026

Reproduced and fixed expiry reversal after clock rollback and acceptance of negative persisted reservation values. Observed expiry now commits revocation; SQLite structure/foreign-key and logical field/state validation runs on open and before writes. Malformed state cannot grant budget even through an already open connection. All 95 evaluation-tooling tests pass; six recovery/concurrency cases pass 20 repetitions each (120 trials). Evidence `evaluation-budget-recovery.json` preserves initial failures. Trusted private storage, genuine approval, request bounds/pricing and actual gateway enforcement remain required. Docker approval remains pending.

### Conservative evaluation request quotes — 9 September 2026

Added exact decimal price contracts and full-context/output-cap quotes using maximum declared input/cache/output rates and explicit request fees. Quotes reject missing/unsupported pricing, model mismatch, excess output and overflow; fractional nano-USD amounts round up. Six tests include 4,550 synthetic token allocations and integration with durable reservation/settlement. All 101 evaluation-tooling tests pass. Evidence `evaluation-request-quotes.json`. Real price/limit verification, payload enforcement, full gateway, paid approval and comparison remain pending; Docker access approval remains unanswered.

### GoClaw feature oracle calibration — 9 September 2026

Calibrated explainable routing against portable GoClaw parent 6a002981f8c40e542e23fd7d86db6ea32999fe98. Initial snapshot required an unpinned sibling Cortex module; retained its infrastructure failures separately. Parent router source/tests are identical. Portable baseline fails compilation for absent Explain; developer patch passes all six tests with race detection and unchanged oracle. Tests cover 24 priority permutations, all constraints, fallback, stable ties and concurrent reads. All 101 evaluation tooling tests pass. Catalogue now five tasks/four repositories, Go/Python, bug-fix/feature, zero held-out. Evidence `goclaw-oracle.json`; M8.1 and final qualification remain incomplete.

### Durable quoted admission — 9 September 2026

Bound exact outbound-body digest and complete canonical pricing contract to each quoted reservation in one transaction. Reopen/write validation recomputes bounds. Five tests cover actual process crash, duplicate identity, injected binding-write failure with rollback, invalid/revoked requests and persisted bound corruption. All 106 evaluation tooling tests pass without warnings after closing two test-owned SQLite connections. Evidence `evaluation-admission.json`. This does not validate provider payloads, forward traffic, authenticate prices/usage or authorise spending; gateway and live comparison remain pending.

### Anthropic request admission policy — 9 September 2026

Implemented bounded strict JSON and Messages-body validation tied to quoted admission. Model/route changes, excess quoted output, unknown billed features, multimodal content and server tools are rejected before reservation. Supported text/custom-tool/cache/thinking fields retain exact body bytes. Five source-informed tests plus full evaluation suite pass (111 total, zero failures/skips). Evidence `evaluation-anthropic-request.json`. Actual Hand/Pi wire compatibility, HTTP/header/beta enforcement, forwarding/isolation, real pricing/usage and paid execution remain pending; unsupported defaults must not be silently removed.

### Actual provider request serialization — 9 September 2026

Captured text, tool-roundtrip and thinking requests from installed Pi AI 0.85.1 and current local Harness, with injected transports returning 400 and no network/model calls. All three Pi requests initially failed the route allowlist because its SDK adds exactly beta=true. Added that exact route without rewriting requests; all six captured bodies now validate and are permanent regression fixtures. Full evaluation suite: 113 tests pass, zero failures/skips/warnings. Evidence `provider-request-capture.json`. This proves the six provider-serialization cases, not full agent sessions, header/network isolation, actual billing or final candidate qualification.

### Evaluation HTTP header policy — 9 September 2026

Added bounded raw-pair validation for authority, content length/type, API version, run credential and explicit beta combinations. Duplicate fields, alternate routing, compressed/chunked request framing and unknown headers are rejected. Upstream projection preserves reviewed SDK headers and removes gateway credentials/hop framing. Six captured SDK header sets pass with injected framing; five focused tests plus all 118 evaluation tooling tests pass. Evidence `evaluation-http-policy.json`. Full HTTP server/forwarding, run revocation integration, joint durable request evidence, network isolation and live accounting remain pending.

### Combined durable HTTP admission — 9 September 2026

Connected header/body validation with quoted budget admission and atomic credential-free transport storage. Fixed upstream origin, exact target, reviewed headers/betas and body digest survive crash alongside quote/reservation. Six actual SDK captures pass; tests verify rejected requests leave no records, injected transport-write failure rolls back everything, actual process crash preserves evidence without replay permission, and digest corruption blocks recovery. All 123 evaluation tests pass, zero failures/skips. Evidence `evaluation-http-admission.json`. HTTP server/forwarding, trusted run configuration, network isolation and real usage reconciliation remain pending.

### Single-use dispatch and bounded sender — 9 September 2026

Dispatch now commits a single-use claim before connection, preserving full reservation after crash. Sender uses fixed HTTPS upstream with verified TLS configuration, unchanged admitted body, no automatic redirects/retries, response cap and connected-socket deadline/cancellation. Nine tests include six real HTTP socket-pair cases, crash recovery, pre-dispatch revocation and TLS settings; all 132 evaluation tests pass, zero failures/skips. Evidence `evaluation-forward.json`. No real provider calls or live TLS qualification. Incoming server, trusted run controller, in-flight revocation wiring, DNS/network isolation and authentic billing remain pending.

### Complete response usage observation — 9 September 2026

Added bounded JSON/SSE usage validation with model identity, final stream framing, explicit disjoint cache counters and nonregressing cumulative updates. Incomplete/unsupported usage remains uncertain. Sender now retains Content-Encoding; encoded bodies require reviewed decoding. Six focused tests plus socket-pair sender/usage integration pass; full evaluation suite 139 tests passes, zero failures/skips. Evidence `evaluation-usage.json`. Parsing never settles a reservation; authentic origin, pricing, reconciliation and live comparison remain pending.

### Trusted gateway run controller — 9 September 2026

Connected trusted token/run settings to validation, durable admission, single-use forwarding and private response evidence. Configuration is snapshotted; client credentials cannot select model/pricing/run identity. Revocation commits before signalling active sockets; close stops new requests and cancels active work. Five socket-pair integrations cover these behaviours and concurrent budget enforcement. All 144 evaluation tests pass, zero failures/skips. Evidence `evaluation-gateway.json`. Incoming server/streaming bridge, full agents, approval/configuration attestation, network isolation and genuine billing remain pending.

### Incoming HTTP and streaming bridge — 9 September 2026

Added bounded authenticated HTTP intake and incremental chunked response bridge. First chunk can reach the client before upstream completion; incomplete upstream omits the terminal chunk. Duplicate length is rejected before dispatch; stalled headers hit an absolute intake deadline. Five HTTP socket-pair integration tests and full evaluation suite pass (149 tests, zero failures/skips). Evidence `evaluation-http-server.json`. Actual TCP/listener lifecycle, SDK network/full-agent integration, deployment isolation and genuine billing remain pending.

### Real loopback listener qualification — 9 September 2026

Three actual TCP tests pass: request/response forwarding, extra-connection refusal without an extra handler, and shutdown joining stalled intake with returned slots. Sandbox initially denied bind; retained those infrastructure failures and obtained elevated execution for local fixture-only tests. Strengthened worker-count and cleanup assertions. All 152 evaluation tooling tests pass, zero failures/skips. Evidence `evaluation-listener.json`. No Docker access, external provider or paid call. Actual SDK network/full-agent integration, deployment isolation and live billing remain pending.

### Actual Pi SDK through TCP gateway — 9 September 2026

Installed Pi AI 0.85.1 now completes text, tool-roundtrip and thinking cases via real guarded loopback fetch and gateway streaming. Initial Node-added accept-language/sec-fetch-mode headers were rejected before reservation; added only observed values and retained before/after evidence plus permanent header fixtures. All three response usage observations validate, all reservations stay uncertain, and server/upstream threads join. Full evaluation suite: 153 tests pass, zero failures/skips. Evidence `pi-gateway-network.json`. Synthetic upstream/pricing only; no real provider, paid call, full coding-agent session or container isolation proof.

### Both provider SDKs through shared TCP gateway — 9 September 2026

Hand's current local Harness provider/Go SDK now completes text, tool-roundtrip and thinking cases through the same guarded loopback gateway. Expected text, end_turn and fixture usage arrive intact; automatic Go transport headers are retained as regression fixtures. Pi's shared-runner branch also passes all three cases after refactoring. All reservations remain uncertain and all workers join. Full evaluation suite: 154 tests pass, zero failures/skips. Evidence `harness-gateway-network.json`. Constructed contexts/synthetic upstream only; no full coding-agent sessions, genuine provider/billing, published Harness or deployment-isolation qualification.

### Real Pi CLI tool turn through gateway — 9 September 2026

Pi 0.85.1 completes two assistant messages and one actual read tool through the loopback gateway. The file marker reaches the second provider request; process exits zero and workers join. Fixed a socket-variable shadowing bug in the initial fixture and retained its failure. Permanent captured-stream regression also rejects missing tool completion. All 155 evaluation tests pass, zero failures/skips. Evidence `pi-agent-gateway.json`. Responses/prices are synthetic and reservations remain uncertain; full Hand sessions, comparison execution, isolation and authentic billing remain pending.

### Actual Hand CLI through gateway — 9 September 2026

Compiled Hand completes read_file and two provider requests through the shared gateway. The second request contains the actual file marker; JSONL ends completed and process exits zero. Strengthened shared runner to require expected request/reservation/usage counts and joined workers. All 155 evaluation tests pass, zero failures/skips. Evidence `hand-agent-gateway.json` retains both initial and strengthened successful probes. Synthetic responses/pricing only; full comparison, deployment isolation, billing and clean candidate qualification remain pending.

### Reusable Hand session observer — 9 September 2026

Added bounded strict JSONL observer for ordering, event uniqueness, stable session/run/request identity, tool pairing and terminal outcomes. Six tests include captured actual CLI replay and negative framing/identity/tool/terminal mutations. Integrated the observer into the Hand gateway probe; fresh actual CLI run passes with eleven events and one successful tool result. Full evaluation suite: 161 tests pass, zero failures/skips. Evidence `hand-observer.json`. Task scoring, process supervision, model identity, billing and full comparison remain independent requirements.

### Supervised Hand evaluator invocation — 9 September 2026

Connected Hand invocation to the bounded process supervisor and strict event observer. A valid terminal cannot override timeout, output cap, failed cleanup or mismatched exit; failed outcomes retain their distinct status. Six regression tests and actual Hand gateway probe pass, with process/pipes joined and 5139 captured bytes. All 167 evaluation tests pass, zero failures/skips. Evidence `hand-supervision.json`. Full comparison integration, Pi supervision, deployment isolation, billing and final candidate qualification remain pending.

### User-approved remaining-scope reduction — 9 September 2026

User requested skipping the listed remaining work except execution isolation and Harness release integration. Recorded `scope-amendment-2026-09-09.md` and linked it from the plan and definition of done. Existing original-scope statuses/evidence are preserved; omitted work is not marked passed. Stop further comparison/corpus/billing development except dependencies needed for isolation. Docker permission is specifically pending; Harness release requires concrete candidate qualification and publication permission.

### Isolation routing regression review — 9 September 2026

Added regression checks that unsupported tools reject installation atomically and all nine worker tool definitions are retained behind the selected container proxy. Corrected a new test compilation error comparing a slice-containing backend value; preserved initial failure. Fresh uncached race tests for isolation/config/toolworker pass 77 records across three packages, zero failures/skips. Evidence `isolation-routing.json`. Actual Docker boundaries and Harness release qualification remain pending; no privileged socket was accessed.

### Harness release qualification gate — 9 September 2026

Found that hosted qualification could accept absent container fixtures via skipped tests. Added a Harness-owned event checker to both full and repeated workflow suites, with five passing regression tests and actual Go-stream replay plus a negative skip mutation. Evidence `harness-qualification-gate.json`. No v0.4.0 public tag currently exists. Source release bundle needs refresh for these additions; Docker/hosted qualification and publication remain pending.

### Refreshed Harness release review bundle — 9 September 2026

Rebuilt the cumulative patch including the qualification event gate. Applied it to an independent historical-base checkout and verified all 302 files, symlinks and executable modes against current source; original worktree/index unchanged. Independent go vet passes. Current fingerprint and patch hashes are recorded in `harness-gate-review.json` and `harness-current-release.md`. Exact-source Docker/native qualification and publication remain pending.

### Native isolation oracle review — 9 September 2026

Required ip/nc availability before accepting network-denial assertions, and added a native child-heartbeat cancellation test requiring cleanup within five seconds and no later writes. Execution tests compile; no Docker tests ran and no isolation pass is claimed. Evidence `isolation-oracles.json`. Review patch needs refresh for these test additions once native qualification can proceed.

### Qualification blocked on explicit Docker access — 9 September 2026

Refreshed the cumulative Harness patch to include native network prerequisites and child-heartbeat cancellation assertions. Independent reconstruction matches all 302 files and go vet passes; current bundle is `harness-isolation-review.json`. Specific Docker permission remains unanswered across repeated turns following automatic review rejection. No qualification process is running. Further release progress depends on real isolation/native qualification, then candidate approval/publication and Hand integration. Blocker recorded in `remaining-scope-blocked.json`; no required outcome is marked complete.

### Docker authorised; first current isolation run — 9 September 2026

User explicitly authorised Docker for these tests. Built a fresh Linux/ARM64 Hand worker; uncached race tests for isolation/config/toolworker/toolproxy pass 80 records with zero failures/skips, including real container worker roundtrip. Evidence `authorised-isolation.json`. Full Harness qualification is running separately; it exposed the native network test’s ip lookup outside restricted PATH, while child-write cancellation passed. No release qualification pass is claimed.

### Authorised Harness Docker qualification completed — 9 September 2026

First run retained: 1113 passing records and one native network-test PATH failure. Corrected test to invoke existing /sbin/ip explicitly; backend PATH unchanged. Targeted native test passed, then fresh full uncached race/coverage suite passed all 1114 records across 32 packages, zero failures/skips/missing/unfinished, source unchanged. Evidence `harness-authorised-corrected.json`. Twenty consecutive native child-write cancellation trials also pass (`authorised-cancellation.json`). Hand isolation/toolworker/proxy suite passes 80 records. No release publication or released-module integration yet.

### Linux Harness qualification started — 9 September 2026

Adapted the existing Linux fixture runner for full Harness vet/race/coverage. First run: 1108 passes, six native-container failures because the non-root runner lacked the mounted Docker socket group; temporary volume cleanup succeeded. Observed Linux socket group 0/mode 0660 and added explicit supplementary-group configuration while retaining non-root UID. Corrected run is active. Evidence `harness-linux-first.json`; no Linux qualification pass claimed yet.

### Linux nested workspace mount correction — 9 September 2026

Socket-group correction resolved daemon access. Second Linux run retained: 1109 passes and five container failures because rprivate mounts cannot originate below Docker daemon storage; volume cleanup succeeded. Runner now uses dedicated host-shared output/tmp outside daemon root, preserving isolation propagation, with an early real-container boundary preflight. Corrected run is live; evidence `harness-linux-mount.json`. No Linux qualification pass claimed.

### Linux Harness passed; local candidate frozen — 9 September 2026

Corrected Linux/ARM64 full suite passes all 1114 test records, no failures/skips, source unchanged; strict event gate also passes. Evidence `harness-linux-qualified.json`. Froze a clean independent candidate at 0972eed3eabe53dd310239bc66643aa89b915bd0 and saved a Git bundle (`harness-frozen-candidate.json`). Working checkout untouched. Fresh macOS and Linux qualification runs are active against that exact commit; no publication performed or authorised.

### Frozen Harness candidate macOS qualification — 9 September 2026

Exact clean candidate 0972eed3eabe53dd310239bc66643aa89b915bd0 passed the full uncached macOS race/coverage suite: 1114 test records, no skipped/failed/missing/unfinished tests; fingerprint unchanged and checkout clean. Evidence `harness-frozen-macos.json`. Linux candidate suite is still live; publication remains unauthorised.

### Frozen candidate Linux qualification completed — 9 September 2026

Exact clean candidate 0972eed3eabe53dd310239bc66643aa89b915bd0 passes Linux/ARM64 full uncached race/coverage: 1114 records, no failures/skips, unchanged fingerprint and clean checkout. Strict event gate passes. Candidate macOS suite also passes 1114 records and all 20 cancellation trials. Evidence `harness-frozen-linux.json` and `harness-frozen-macos.json`. Hosted workflow needs fixture provisioning before it can qualify these same native tests; publication and Hand released-version integration remain open.

### Reproducible CI fixture preparation — 9 September 2026

Added Harness-owned fixture preparer accepting loaded immutable image IDs and an explicit local socket. It validates Linux architecture consistency, builds an untagged implicit-volume fixture without networking and records exact environment settings. Actual Harness native implicit-volume rejection test passes with the prepared image. Evidence `harness-fixture-preparer.json`. Workflow daemon/bootstrap integration remains pending; this helper is newer than the frozen candidate and must be included in its next revision.

### Harness CI fixture wiring — 9 September 2026

Workflow now prepares Docker fixtures before native suites, records resolved image IDs/daemon version and exports the exact environment. Shell step passes bash syntax validation; real helper and negative fixture were tested previously. Added explicit runner/reproduction requirements. Hosted macOS daemon provisioning remains an external prerequisite; no hosted pass claimed. Evidence `harness-ci-wiring.json`. Workflow/helper additions need candidate revision before publication.

### CI fixture step executed; candidate revised — 10 September 2026

Executed the actual workflow fixture shell step in a fresh directory; it pulled/resolved base images and exported the exact fixture environment. All 45 execution/bash test records pass with these settings, no failures/skips. YAML parsing and shell syntax checks pass. Added an ignore exception so qualification documentation is included. Revised local candidate cc756425e8b0c2366b08d33f3184cf9ede3e6f7e includes CI provisioning; a Git bundle is saved. Previous full platform results remain tied to 0972eed, not relabelled. Evidence `harness-ci-executed.json`; final revised-candidate verification and publication remain pending.

### Revised candidate qualification and release review — 10 September 2026

Verified candidate cc756425e8b0c2366b08d33f3184cf9ede3e6f7e is clean. Started fresh full macOS and Linux Docker/race/coverage runs against that exact checkout; handles and output directories are recorded in `harness-frozen-candidate.json`. Prepared `harness-release-review-2026-09-10.md` with exact identity, consumer migration, rollback and publication/integration steps. Hosted macOS setup and public release permissions remain explicit prerequisites, not fabricated passes.

### Revised candidate passed both native suites — 10 September 2026

Candidate cc756425e8b0c2366b08d33f3184cf9ede3e6f7e passes fresh macOS and Linux suites with 1114 records each, zero failures/skips; 20 native cancellation trials pass. Source fingerprints agree and candidate remains clean. Archived combined evidence in `harness-revised-qualified.json`; release review updated. Next external steps require permission to push/review/publish Harness; hosted macOS Docker runner setup remains explicit. Hand released-module integration follows publication.
