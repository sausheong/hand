# Hand implementation plan: dependable terminal work and automation

> Remaining scope was explicitly revised by the user on 9 September 2026 to execution isolation and Harness release integration. See the [scope amendment](acceptance/scope-amendment-2026-09-09.md). The original requirements below remain historical; omitted work is not a pass.

Date: 7 September 2026  
Status: proposed delivery plan; implementation has not started  
Baseline: Hand `e6dd263`, Go `1.25.1`, Harness `v0.3.9`

Completion contract (added 8 September 2026): [definition of done](acceptance/definition-of-done.md), [test coverage matrix](acceptance/test-matrix.md) and [machine-readable requirements](acceptance/requirements.json). These define the evidence required to close this plan: 32 requirement groups, 221 scenarios, quantitative quality gates and a final acceptance review. The [completion checker](acceptance/check.py) validates evidence; it does not implement or run the future product tests.

The completion contract makes the original acceptance prose testable. Its fixed thresholds supersede the provisional thresholds below. Paid/live evaluation may remain pending at an **offline-ready** checkpoint, but the full implementation goal cannot be marked complete until the live gate passes. A published Hand release and a claim of superiority to Pi are separate from implementation completion.

## 1. Objective and scope

Deliver the recommendations from the September 2026 project review. Make Hand a dependable, portable coding agent with recoverable sessions, precise permissions, trustworthy usage accounting and a stable integration interface. Retain the small Go application and optional integrations rather than requiring a daemon.

Success means users can interrupt, resume, inspect, automate and extend real coding work without losing their changes or misunderstanding what the agent did. Claims of superiority to Pi must be supported by comparative evaluation; implementing this plan alone does not establish superiority.

This document is the implementation backlog and sequencing contract. It does not authorise paid evaluations, publish releases, or change application behaviour. Future implementation should proceed through the work packages below, with each package producing code, tests, documentation and reviewable evidence.

The existing [Phase 7 plan](superpowers/plans/2026-09-05-hand-phase7-plan.md) and other Phase 1–7 documents remain historical records. Their descriptions of the then-current implementation are not the present baseline. This plan deliberately expands earlier exclusions concerning per-tool permissions, MCP cancellation, richer sessions and extensibility. Do not renumber or rewrite those documents.

### Baseline evidence

- The review ran `go test -race ./...` and `go vet ./...` successfully. This is historical baseline evidence, not validation of future changes.
- Temporary Go overlay probes confirmed incomplete large-file search, ungated skill mutation, an in-memory permission surviving failed persistence, and acceptance of stale goal-loop results. These probes are not checked-in regression tests; M1 must add permanent tests.
- The review inspected relevant Harness implementations, but did not benchmark model task quality, latency, memory or cost against Pi.
- Hand currently has a tag-triggered release workflow and no PR validation workflow.

### Delivery principles

1. Fix correctness before expanding autonomy.
2. Keep reusable execution, provider and persistence mechanisms in Harness; keep Hand product policy, interaction and integration contracts in Hand.
3. Keep existing user data and unrelated workspace edits intact during migrations and recovery.
4. Implement one end-to-end slice before generalising it. Public APIs stabilise only after both CLI and TUI exercise them.
5. Preserve text one-shot usage while adding structured interfaces. Advertise intentional behaviour changes, particularly stricter permissions and nonzero incomplete-run exits.
6. Treat every estimate and performance threshold below as a planning assumption to validate, not a measured result.

## 2. Target architecture and ownership

```text
TUI / text CLI / JSONL CLI / RPC / Go SDK
                   |
            Hand application service
     sessions, runs, queues, outcomes, policies
          |                       |
    Harness adapter       extensions / checkpoints
          |
 Harness runtime, providers, tools, process execution,
          session persistence and compaction
```

The TUI must eventually render state and submit commands without writing fields on `runtime.Runtime`. The application service owns lifecycle transitions, active session identity, provider configuration, cancellation, approval requests and event ordering.

| Component | Owner | Starting points / proposed location |
|---|---|---|
| Application state machine and event contracts | Hand | New `internal/app`; migrate coordination from `cmd/hand/main.go`, `internal/tui/model.go` and `controller.go` |
| Runtime integration and capability translation | Hand | Existing `internal/agentio`; keep Harness types behind the adapter where practical |
| Sessions, selection and migration | Hand + Harness | Hand `internal/sessionio`; Harness `session` for durable storage primitives |
| Provider profiles and user settings | Hand | `internal/config`; Harness owns request translation and capability enforcement |
| Policy decisions and grants | Hand | `internal/permissions`; Harness tools receive execution constraints |
| Process lifecycle, bounded capture, cancellable MCP and file writes | Harness | `tools/bash`, `tools/mcp`, runtime builder and `tools/file` |
| TUI message blocks, viewers and input | Hand | `internal/tui` |
| Public integration surfaces | Hand | Proposed `sdk`, `internal/rpc`, checked-in protocol schemas |
| Extension host and installation | Hand | Proposed `internal/extensions`, protocol schemas and examples |
| Workspace checkpoints and verification records | Hand | Proposed `internal/checkpoints` and `internal/verification` |

Names for new packages are proposed, not existing APIs. Avoid exposing mutable Harness runtime pointers through the public SDK. Reusable library changes require a published Harness version before their dependent Hand release; release builds must not rely on a local `replace` directive.

## 3. Milestones and dependencies

Estimates are engineer-weeks including implementation, tests, documentation and review, assuming one experienced Go engineer with access to both repositories. They exclude external release delays and paid model evaluation. Total initial estimate: **32–49 engineer-weeks**. Re-estimate after M1 and M3; additional engineers do not reduce the critical path proportionally.

| Milestone | Outcome | Dependencies | Effort |
|---|---|---|---|
| M0 | Baseline, CI and delivery contracts | None | 1 |
| M1 | Correctness and execution hardening | M0; Harness releases for shared fixes | 4–6 |
| M2 | Shared application service and provider profiles | M1 lifecycle/outcome contracts | 3–5 |
| M3 | Durable sessions and usable interaction | M2 | 5–7 |
| M4 | JSONL, RPC and Go SDK | M2; final session commands need M3 | 3–4 |
| M5 | Precise policy, isolation and recoverable edits | M3, M4, M1 process backend | 6–9 |
| M6 | Inspectable context and enforceable budgets | M2 accounting; M3 views; M4 events | 4–6 |
| M7 | Reloadable extensions and packages | M4, M5 policy contract, M6 context hooks | 4–7 |
| M8 | Comparative evaluation and release qualification | Evaluation fixtures begin at M0; final gate follows M7 | 2–4 |

M4 protocol design can begin during M3. M5 and M6 can progress independently after their prerequisites, with separately owned files and integration contracts. This describes dependency opportunities, not a requirement to use multiple agents.

Suggested release checkpoints: a correctness release after M1, a daily-use beta after M3, an automation beta after M4, and a differentiated release after M8. Use actual version numbers only when preparing those releases.

## 4. M0 — Baseline and release discipline

### M0.1 Establish reproducible checks

- Add a PR/push workflow running formatting checks, build, vet and race-enabled tests. Test Linux and macOS; cross-compile every advertised release target.
- Add a Harness integration contract test group covering model switching, usage events, cancellation, persistence, tool permissions and MCP registration. Do not duplicate Harness's entire suite.
- Make the release workflow depend on successful validation of the same commit it publishes. Inject version and commit metadata into the binary, add `--version`, and remove the hard-coded banner version.
- Include licence and checksum files in release artefacts. Smoke-test archive extraction and `--help`/`--version` without credentials. Document the shell dependency of command execution and any optional extension runtimes.

**Acceptance:** a deliberately failing test prevents release publication; all advertised archives identify their source commit; clean checkout commands reproduce CI.

### M0.2 Establish measurements and decision records

- Add offline fixtures for long transcripts, large repositories, slow/hung tools and fake providers with controlled usage/error streams.
- Record binary size, startup-to-input latency and resident memory without MCP, then with fast, slow and unavailable MCP servers.
- Create an evaluation manifest format capturing repository commit, task, tool configuration, model settings, budgets and verification commands.
- Record architecture decisions for lifecycle state, protocol versioning, session migration, execution isolation and extension permissions as each is settled.

**Acceptance:** measurements can be rerun without API keys; benchmark artefacts identify platform and revision; no performance claim is based on language choice alone.

## 5. M1 — Correctness and execution hardening

### M1.1 Separate context occupancy from consumption

Starting points: `internal/tui/status.go`, `internal/agentio/compaction.go`, Harness runtime usage and provider adapters.

- Expose latest-request input usage separately from cumulative run usage. Define a provider-neutral accounting contract so cached input is not double-counted.
- Include separate call categories for normal generation, retry/fallback and compaction; distinguish reported values from estimates and unavailable data.
- Make the context gauge use the latest request for the active model. Reset or mark it unknown when the active model/session changes; do not retain an old denominator.
- Persist usage records later through M3; expose cost and admission budgets in M6.

**Acceptance:** ten 20k-input requests show approximately 20k latest-request context and 200k cumulative input, not 200k context. Tests cover cache semantics, missing usage, failed requests, model switches and compaction. Failed billable requests with unavailable usage remain explicitly unknown.

### M1.2 Make search completeness explicit

Starting point: `internal/agentio/search.go`.

- Replace the 64 KiB exclusion with streaming scanning, bounded line handling, total output/byte limits and cancellation checks.
- Support `.gitignore` semantics, including nested rules and negations, with explicit overrides for ignored/hidden files. Choose a maintained parser or an optional `rg` backend with a tested Go fallback; the binary must retain useful search without `rg`.
- Return structured match metadata plus `truncated`, `cancelled`, skipped-file reasons and filesystem errors. Invalid globs are errors. Do not claim an exhaustive no-match result after partial traversal.

**Acceptance:** a match in a source file larger than 64 KiB is found; result limits and unreadable files are visible; cancellation is prompt; tests cover nested ignore rules, binaries, long lines and symlink boundaries.

### M1.3 Repair approval persistence and operation classification

Starting points: `internal/agentio/approval.go`, `registry.go`, `internal/permissions`.

- Classify each operation's effects, including `skill_manage` create/patch/replace/remove and persistent todo mutations. Keep read/list operations separately classifiable.
- Gate skill mutations in interactive and one-shot modes. Inventory every registered tool so new tools cannot silently inherit an unknown permissive classification.
- Save grants atomically before updating memory. A failed save allows only the already-approved invocation and emits a visible persistence failure; it does not install an implicit continuing grant.
- Add regression tests for the temporary review probes. Full grant scope and migration arrive in M5.

**Acceptance:** no skill mutation without applicable approval; failed saves cause the next invocation to prompt again; tool registration tests enforce complete classification.

### M1.4 Correct run completion and asynchronous checks

Starting points: `internal/tui/model.go`, `events.go`, `commands.go`, `internal/agentio/hooks.go`, `cmd/hand/main.go`.

- Attach session ID, run ID and generation to all asynchronous results. Reject results from superseded runs, including after `/new`, model changes, explicit cancellation and newer user work.
- Represent goal-check and compaction activity explicitly. Run manual compaction off the update goroutine under a cancellable context.
- Define completion outcomes: `completed`, `verification_failed`, `budget_exhausted`, `cancelled`, `infrastructure_error`. Carry the triggering reason separately, such as `max_iterations` or `max_turns`.
- Return nonzero exits when limits are exhausted or mandatory verification cannot establish completion. Reserve a documented mapping: 0 completed, 2 invalid invocation/configuration, 3 verification failed, 4 budget/iteration limit, 5 infrastructure failure, 130 user interrupt.
- Keep run completion separate from evidence of correctness: an ordinary model answer is not a verified coding success.
- Replace bulk hook environment variables with versioned JSON stdin. Keep small metadata variables temporarily for compatibility. Add bounded output and explicit `failure_policy`; mandatory validators deny on failure, optional observers may warn and continue.
- Add `signal.NotifyContext` or equivalent process-wide cancellation for one-shot mode and orderly shutdown.

**Acceptance:** a delayed Stop hook cannot launch work in a new session; a user can cancel compaction; iteration exhaustion returns 4; validator timeout cannot produce verified success; large hook payloads do not depend on OS environment-size limits. Legacy hook migration is documented and tested.

### M1.5 Fix execution mechanisms in Harness

- Add cancellable runtime/MCP construction with propagated contexts and cleanup of partially connected servers. Replace Hand's abandoned-goroutine timeout wrapper when the new API is released.
- Bound stdout/stderr while capturing them, spill larger results to controlled artefact files, and include truncation/location metadata. Bound disk capture and retention too.
- Terminate the subprocess group/tree on cancellation, close inherited pipes and reap children. Document and test platform-specific behaviour on advertised operating systems.
- Preserve existing file permissions when `write_file` replaces a file; retain restrictive creation defaults for new files. Test executable scripts, failed replacement and atomic-write behaviour.

**Acceptance:** hung MCP handshakes and shell grandchildren are cleaned up; unbounded-output fixtures cannot grow memory indefinitely; overwriting an executable preserves its executable bits. Hand integration tests pass against a released Harness version.

## 6. M2 — Shared application service and model profiles

### M2.1 Extract lifecycle ownership

- Introduce `internal/app` with explicit states: idle, running, awaiting approval, checking completion, compacting and cancelling. Model state transitions independently from Bubble Tea messages.
- Move runtime coordination, goal loops, provider switches and permission request lifetimes out of the TUI and `main.go` into the service.
- Introduce immutable application events with session/run IDs, sequence numbers and timestamps. A run emits exactly one terminal outcome after tool and persistence cleanup.
- Retain an adapter around Harness. Switch models only at defined safe boundaries after dependent compaction has settled; construct replacement configuration before committing the switch.

**Acceptance:** TUI and text CLI use the same lifecycle operations; neither changes runtime fields directly. Fake-provider tests prove event ordering, cancellation, terminal-outcome uniqueness and rejection of invalid concurrent operations.

### M2.2 Introduce provider/model profiles

- Profiles own endpoint, credential reference, model ID, supported input types, context limit, maximum output and reasoning capability. Keep credentials in environment/key references rather than duplicating them in profile exports.
- Migrate the existing `model` and `base_url` pair into an equivalent default profile. Never forward one profile's endpoint or credential to another implicitly.
- Surface reasoning and explicit context limits through config, flags and a model picker; unsupported settings produce actionable errors.
- Resolve metadata in order: explicit user override, verified provider/server metadata, versioned catalogue, labelled conservative fallback. Distinguish an advertised maximum from a local server's configured active context.
- Preserve same-provider fallback behaviour initially, and record actual serving model/profile in events when known. Do not represent an aggregator alias as independently verified underlying model identity.

**Acceptance:** switching local → hosted → custom proxy routes to the intended endpoints; failed switches leave the original profile intact; compaction uses the selected profile; UI and runtime use the same resolved context value.

## 7. M3 — Durable sessions and everyday interaction

### M3.1 Session catalogue and safe migration

- Separate workspace identity from session identity. Store multiple named sessions per canonical workspace, with timestamps, selected branch, model changes, outcomes and usage metadata.
- Import existing workspace-hash JSONL sessions without deleting or rewriting the only copy. Back up originals, validate import and make migration idempotent. Define schema versions and reject unsupported future versions.
- Make `/new` and `--new-session` create a new session. Add `/resume`, `/name`, `/fork`, `/tree`, explicit session selection and JSONL export. Existing auto-resume selects the last active session unless overridden.
- Use Harness DAG primitives after testing branch persistence across restart and compaction. Persist the selected leaf explicitly; appending records alone must not accidentally change it on reload.
- Add an exclusive writer lease/OS lock per session with stale-owner recovery. A second process must select another session or explicitly take over an inactive lease.
- Detect and recover a truncated final record without silently dropping arbitrary corrupt interior records. Surface degraded persistence as product state, not only a log warning.
- Persist usage and attachment references; bound exported record sizes and avoid inline images exceeding the reader's limits.

**Acceptance:** old sessions survive migration, interrupted migration is recoverable, repeated migration does not duplicate sessions, branches resume correctly, `/new` preserves history, and two processes cannot write one session accidentally. Tests include disk-full/permission failures and process termination during append.

### M3.2 Steering, follow-ups and editing input

- Add separate steering and follow-up queues with visible state and editable/cancellable queued messages.
- Deliver steering at a safe tool boundary before remaining eligible work; deliver follow-ups after the active run and its completion check settle. Preserve queue intent through cancel/retry without duplicating prompts.
- Add `@` file references, path completion, an external editor and robust image/file attachment parsing, including paths with spaces. Show attachment errors instead of silently omitting files.
- Resolve explicit user-selected external attachments through policy; do not broaden automatic file access.

**Acceptance:** users can correct a running agent without losing input; queue order is deterministic; cancellation restores pending input predictably; UI tests cover approval/steering collisions and multiline editing.

### M3.3 Structured transcript and viewers

- Store typed message/tool/approval/compaction blocks with raw content and presentation metadata, replacing rendered strings as the source of truth.
- Add expandable full tool output, search/copy within output, and a dedicated scrollable diff viewer with proper separated hunks. Keep truncated/captured-to-file results clearly labelled.
- Bound approval panel height independently of diff size. Show the resource/command being approved and preserve terminal sanitisation across live and replay paths.
- Render visible blocks incrementally; coalesce text deltas and cache layout by width. Reflow correctly on resize without repeatedly wrapping the entire history.
- Show connecting/degraded MCP servers, pending verification, compaction progress and persistence failures.

**Acceptance:** a long diff remains navigable on an 80×24 terminal; full output is available; replay and live rendering agree. Initial offline target: p95 key-to-render under 100 ms for a 10,000-block fixture on the recorded reference machine. Measure against M0 before accepting the target.

### M3.4 Managed background commands and MCP startup

- Expose process handles for background servers and tests: start, stream/read, send input where supported, cancel and await completion. Use the M1 bounded-capture and process-tree backend.
- Define ownership: foreground cancellation stops owned foreground work; deliberately backgrounded work is visible and stopped on shutdown by default. Do not advertise process survival across Hand restart without a separately designed supervisor.
- Start the TUI before optional MCP connections complete. Connect independently with per-server deadlines, retry/status controls and an explicit required/optional setting. Required-server failure prevents dependent runs; optional failure leaves local work usable.

**Acceptance:** a development server can run while the agent works and can be stopped reliably; failed MCP servers do not hide the interface or create an unbounded startup delay; tool availability changes are consistent between model context and UI.

## 8. M4 — Integration contracts

### M4.1 JSONL and RPC

- Add JSONL event output with no human text on stdout; diagnostics stay on stderr. Preserve existing text output as the default.
- Publish a versioned event envelope with schema version, event ID, sequence, session/run/request IDs, timestamp and payload. Include tool starts/results, approvals, usage, state, artefacts and terminal outcomes.
- Add bidirectional stdio RPC for session selection, prompt submission, steering, follow-ups, approval decisions, cancellation, profile changes and state/event retrieval.
- Define capability negotiation, unsupported-version errors, maximum message sizes, output backpressure, disconnect cleanup and event retention/replay limits. Never drop terminal outcomes silently.
- Support idempotent prompt requests: replaying a request ID returns its run instead of launching duplicate work. Do not automatically replay side-effecting tools after a crash; report uncertain completion for reconciliation.
- Noninteractive clients cannot hang on a hidden stdin trust prompt. Approvals arrive through the selected interface or terminate with a documented unresolved-approval outcome.

**Acceptance:** a small non-Go client starts and resumes a session, handles an approval, steers, cancels and receives one terminal result. Protocol fixtures cover duplicates, malformed input, slow consumers and disconnect during a tool call.

### M4.2 Public Go SDK

- Expose immutable request/result/event types and context-based cancellation through `sdk` with examples for embedding and RPC clients.
- Keep internal package and Harness implementation details out of exported signatures wherever feasible. Document concurrency, ownership, cleanup and compatibility guarantees.
- Drive a real CLI workflow and a minimal embedded example through the SDK before declaring its API stable.

**Acceptance:** an external Go module can run the examples using released packages; SDK and RPC clients observe equivalent lifecycle semantics; compatibility fixtures run in CI.

## 9. M5 — Precise policy, isolated execution and recoverable changes

### M5.1 Scoped policy and grant migration

- Represent grants by operation, resource scope, workspace, invocation/session/persistent lifetime and provenance. Include file reads/writes, skill mutations, commands, network destinations, external paths and MCP server/tool identities.
- Store approval authority outside agent-writable project files. Treat project policy as requested capabilities, not self-authorising grants. Bind persisted decisions to canonical workspace and relevant configuration identity; changed capability scope requires a new decision.
- Migrate legacy per-tool `always_allow` entries visibly. Preserve their broad meaning only after explicit acknowledgement; do not silently reinterpret `bash` as a narrower safe grant or silently extend trust to changed settings.
- Provide permission inspection and revocation. Approval previews must match executed arguments and changed file content; revalidate stale previews before mutation.
- Do not rely on naive shell prefix matching to enforce safety. Prefer structured executable/argument operations; arbitrary shell commands require a clearly scoped grant and/or an enforced execution backend.
- Document defaults for routine workspace edits and tests. In unrestricted execution, do not claim that approving a test command confines arbitrary code executed by that test.

**Acceptance:** representative workspace edits need few repeated prompts under the selected policy; broader paths/network/tool capabilities remain explicit; symlink/config-change/revocation cases cannot silently expand authority.

### M5.2 Optional isolation backend

- Define an execution-backend interface and implement a container backend first, alongside an explicitly labelled unrestricted host backend. Go portability does not imply uniform native OS sandboxing.
- Scope workspace mounts, writable scratch space, credentials/environment and network access. Route built-in tools and extension host operations through the same policy where applicable.
- Never silently fall back to unrestricted execution when isolation is requested but unavailable. Provide clear setup diagnostics and expose effective boundaries in UI/RPC.
- Run local MCP/extensions inside the backend when isolation is requested and supported. Otherwise label them as external to that boundary and require explicit trust. Remote MCP effects remain controlled by remote credentials and approval, not by a local container.

**Acceptance:** adversarial fixtures cannot read/write outside allowed mounts or use disabled networking; child processes inherit restrictions; missing container support fails clearly. Document host/container differences on Linux and macOS.

### M5.3 Checkpoints and verification records

- Record per-turn file before/after content hashes, modes and change provenance in a bounded content store. Capture the user's starting dirty tree without resetting it; support untracked files and non-Git workspaces with explicit size limits.
- Implement `/changes` and selective restore with a dry-run preview. Restore only when current content matches the recorded post-image; otherwise present a conflict/three-way merge. Never use unconditional `git reset --hard` or blanket stash restoration.
- Keep conversation navigation separate from file restoration. Forking a session does not silently roll back the workspace.
- Attach verification records containing command/profile, working directory, exit status, artefact references and the exact workspace snapshot/digest tested. Later edits invalidate or mark affected evidence stale.
- Display final states such as changed/unverified, tests passed and verification failed. A test result is evidence for its stated scope, not proof of all correctness.
- Define retention, size limits and deletion/export behaviour for captured file content, including handling of sensitive or excluded paths.

**Acceptance:** restore preserves pre-existing and subsequent user edits; mode changes and conflicts are handled; a passing test on an earlier snapshot cannot label later changes verified. External effects such as deployments are explicitly outside file rollback.

## 10. M6 — Context engineering and budgets

### M6.1 Instruction and skill discovery

- Load personal instructions, ancestor instructions down to the workspace, and directory-specific instructions lazily when relevant files are accessed. Define deterministic precedence: user request, nearer applicable project guidance, broader project guidance; Hand-specific and AGENTS guidance at one level need an explicit documented rule.
- Preserve existing `.hand/skills` discovery and add conventional `.agents/skills` locations. Report collisions and source paths; avoid silently changing which skill wins during migration.
- Return skill bodies with their resource base directory so relative references resolve correctly. Refresh the index after self-authoring and through `/reload` at safe boundaries.
- Bound discovery and instruction size, handle unreadable files visibly, and include source provenance in context inspection. Instruction loading must not grant executable capabilities.

**Acceptance:** nested projects receive the right instructions; relative skill resources resolve; a newly authored skill appears without restart; source collisions and size exclusions are visible.

### M6.2 Context inspection and compaction controls

- Add `/context` and RPC equivalents showing instruction sources, skills, tool schemas, messages, large results and estimated token contribution. Distinguish estimates from provider-reported totals.
- Support pinned objectives/constraints and user compaction instructions. Preserve structured objectives, decisions, unresolved work and verification references across summaries.
- Allow a separate summarisation profile and budget, with clear disclosure when it changes the data destination. Retain automatic/reactive compaction and preserve tool-call/result pairing.
- Keep raw history recoverable after compaction; use artefact references for large outputs and lazy tool disclosure when schema volume is material.
- Evaluate the 40% default against observed task quality and cost before changing it. Different context sizes and workloads may need different policies.

**Acceptance:** context inspection explains major contributors; pinned constraints survive repeated compaction fixtures; summariser failure leaves valid recoverable history; tool results remain retrievable; compaction cost is visible.

### M6.3 Admission budgets and accounting

- Add run/session token, cost and elapsed-time limits. Persist ledger entries per request, including retries, fallback and compaction; record currency/pricing source/version and unknown charges.
- Reserve estimated worst-case request cost using input estimates and output caps before dispatch, then reconcile reported usage. Share reservations across concurrent operations.
- In strict cost mode, unknown pricing or unbounded provider charges block admission; in advisory mode, show uncertainty. State that provider billing lag, estimation error and non-cancellable in-flight requests can prevent an exact invoice cap.
- Stop new work and cancel owned work when time/token/cost limits are reached; emit `budget_exhausted` with reason and resumable state. Require a new explicit budget decision before continuing a strict exhausted budget.
- Expose cache effectiveness, per-run/session totals and compaction spending. Restart does not reset the persisted session ledger.

**Acceptance:** retries and summaries cannot bypass budget accounting; concurrent requests cannot reserve the same remaining balance; exhausted budgets produce the documented outcome; tests cover unknown pricing, missing usage and restart.

## 11. M7 — Extensions and distribution

### M7.1 Persistent subprocess extension protocol

- Build a versioned stdio protocol separate from MCP's tool protocol. Support command registration, lifecycle subscriptions, context transformations, structured user questions, tool presentation and extension-owned session state.
- Keep processes persistent for the session, with cancellation, health/status, bounded output and deterministic shutdown. Order context transformations explicitly; errors from mandatory policy hooks cannot fail open.
- Route file/network/process access requested through the host through policy. A host-launched arbitrary subprocess still has OS permissions unless isolated: capability declarations alone are not a sandbox.
- Represent custom presentation through declarative blocks rendered by Hand; avoid arbitrary terminal escape output. Define precedence so extensions cannot bypass user policy or overwrite terminal outcomes.
- Implement `/reload` transactionally at safe boundaries. A failed reload leaves the previous working extension active; restart only changed components where possible.

**Acceptance:** a small Go extension and a Python extension demonstrate a command, user interaction, context transformation and persisted state. Reload and crash fixtures leave the core usable and retain policy boundaries.

### M7.2 Package installation and examples

- Package skills, prompts, extension executables/source metadata and presentation assets with a manifest declaring version, compatibility and capabilities.
- Start with explicit local/Git/archive installation and a lockfile pinned to commit/digest. Add list/update/remove commands, reviewable capability changes and reproducible rollback.
- Never execute project-local extensions merely because a repository contains them. Installation and first execution use the configured trust policy. Identify extra language/runtime dependencies before activation.
- Provide examples: verification hook, custom command, context filter, structured question and tool viewer. Explain when MCP is sufficient and when an extension is needed.
- Include protocol conformance and package integrity checks in CI. No public marketplace service is required for this milestone.

**Acceptance:** another user can install a pinned example reproducibly, inspect its capabilities, reload it and remove it without editing Hand source. Updates changing privileges require a new decision.

### Conditional follow-on: WASM

The review recommended evaluating WASM only after demand is demonstrated. M7 delivers an architecture decision recording extension use cases that subprocesses cannot satisfy, runtime/library cost, startup/memory measurements and isolation semantics. Implementing a WASM host is not required to complete this plan unless that decision establishes a concrete requirement. Native OS sandboxes beyond the container backend and a public marketplace are similarly separate extensions, not hidden prerequisites.

## 12. M8 — Comparative evaluation and release qualification

### M8.1 Evaluation design

- Maintain at least 30 initial tasks across bug fixes, small features, refactors, tests, repository discovery and long-session continuation. Include different languages and repository sizes. Keep a held-out subset out of prompt/tool tuning.
- Run offline fault scenarios for cancellation, interrupted persistence, invalid tool calls, slow MCP, output flooding, stale approvals and budget exhaustion.
- For paid comparisons, pin both Hand and Pi revisions, repository snapshots, accessible tools, provider/model settings, reasoning/output limits and budgets. Compare default experiences separately from reasonably configured workflows.
- Use repeated paired runs for stochastic tasks. Record task success verified by tests/review, regressions, cost per successful task, completion time, human interventions and recovery success. Publish failures and uncertainty, not only averages.
- Obtain a concrete paid-run budget before execution; the plan does not specify or spend that budget. If unavailable, finish offline qualification and leave comparative superiority unclaimed.

### M8.2 Decision and release gates

- Mandatory: all M1 regressions remain fixed, migrations/recovery pass, policy/isolation tests pass, supported packages build, and protocol compatibility fixtures pass.
- Proposed performance gates: cancellation initiates within one second in controlled fixtures; children terminate within the configured grace period; UI meets the M3 responsiveness target. Baseline measurements may justify revised targets documented before final evaluation.
- Select a superiority claim only for a measured dimension with a stated task set and uncertainty. For example, lower interventions at comparable verified task success, or better recovery in interruption scenarios. Do not claim overall superiority from a small or mixed result set.
- Publish installation, migration, session/restore, policy, automation, SDK, extension and troubleshooting guides. Include known limitations and reproducible evaluation commands.
- Prepare checksummed release artefacts from the validated commit, retain the prior release and document rollback compatibility. Publishing is a separate release action after a concrete release candidate exists.

**Acceptance:** a release evidence bundle identifies code versions, checks, task manifests, outcomes, unresolved limitations and migration/rollback instructions. Paid comparison is either completed within its authorised budget or explicitly pending; absence of it does not become a fabricated success claim.

## 13. Recommendation coverage

| Review recommendation | Delivery work |
|---|---|
| Correct context gauge and consumption accounting | M1.1, M6.3 |
| Complete, bounded, ignore-aware search | M1.2 |
| Skill approval and failed-save repair | M1.3 |
| Stale goal results and responsive manual compaction | M1.4, M2.1 |
| Provider-scoped endpoints, reasoning and model metadata | M2.2 |
| Reliable completion exits and structured hook input | M1.4, M4.1 |
| Bounded Bash output, child cleanup and executable modes | M1.5 |
| Real MCP cancellation and usable startup | M1.5, M3.4 |
| Non-destructive named sessions, branches, export and recovery | M3.1 |
| Session usage, process ownership and corruption handling | M1.1, M3.1 |
| Shared application layer, JSONL/RPC and public SDK | M2.1, M4 |
| Precise grants, inspection, revocation and enforcement | M5.1, M5.2 |
| Workspace checkpoints, selective restore and test evidence | M5.3 |
| Steering, follow-ups, file references and external editor | M3.2 |
| Full output, usable diffs, structured/incremental rendering | M3.3 |
| Background processes and visible operational states | M3.3, M3.4 |
| Hierarchical instructions, resource-aware skills and refresh | M6.1 |
| Context inspector, pinned constraints and summariser choice | M6.2 |
| Token/cost/time budgets including retries and summaries | M6.3 |
| Rich extensions, reload, package pinning and capabilities | M7.1, M7.2 |
| Conditional WASM assessment | M7 conditional follow-on |
| Release checks, version metadata and distribution evidence | M0.1, M8.2 |
| Fair comparison with Pi and evidence-backed positioning | M0.2, M8.1, M8.2 |

## 14. Risks, migration and working procedure

### Main risks and mitigations

| Risk | Mitigation |
|---|---|
| Shared fixes turn into Hand-specific forks of Harness | Release reusable mechanisms in Harness first and pin them with Hand contract tests. |
| Lifecycle extraction becomes a rewrite | Move one operation at a time behind the application service; run existing TUI replay/interaction tests throughout. |
| Session migration or restoration loses work | Backups, idempotent imports, writer ownership, hash-checked restoration and crash/failure fixtures. |
| Permissions reduce usability or imply false isolation | Measure interventions; distinguish grants from OS enforcement; show effective backend and capability scope. |
| Broader extension power bypasses core controls | Versioned host API, trust before activation, bounded lifecycle and isolated execution where claimed. |
| Usage looks precise despite incomplete provider data | Preserve unknown/estimated states and pricing provenance; strict budgets refuse unpriceable admission. |
| Scope grows faster than quality | Milestone exit gates and usable releases; defer only the explicit conditional follow-ons, not accepted core recommendations. |

### Migration contracts

- Configuration migration preserves legacy model/endpoint behaviour in a named profile and backs up the old file before first write.
- Session migration preserves the old log and supports recovery; older binaries must not silently write a newer incompatible schema. Rollback uses the original backup or a supported export, not an assumed downgrade.
- Permission migration exposes existing broad grants and moves authority outside the workspace. Tests cover changed settings after initial trust.
- Hooks retain documented compatibility metadata during a deprecation period; JSON stdin is the new authoritative payload.
- Protocol and extension breaking changes require version negotiation and compatibility notes. Lockfiles keep updates deliberate.

### Per-package completion checklist

1. Inspect the current code and revalidate the package's baseline assumptions.
2. Add a regression/contract fixture for the behaviour being changed, where meaningful.
3. Implement in the owning repository, including any required Harness release and Hand dependency update.
4. Run targeted tests, then build, vet and race-enabled tests for affected modules; run platform/process fixtures when relevant.
5. Update user documentation and migration notes. Record measured results, trade-offs and unresolved external gates.
6. Mark the work package complete only when its acceptance criteria are demonstrated. A mocked provider proves application behaviour, not real-model task quality.

The first implementation slice is M0.1 followed by M1.1–M1.4. It establishes trustworthy signals and lifecycle behaviour before new autonomy or public APIs depend on them.

## 15. References

- [Hand README](../README.md)
- [Existing implementation plans](superpowers/plans/)
- [Existing design specifications](superpowers/specs/)
- [Pi product and design philosophy](https://pi.dev/)
- [Pi coding-agent documentation](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/README.md)
- [Pi extension documentation](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md)
- [Pi RPC documentation](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc.md)

Pi references informed the preceding review. Pin and refresh the competitor implementation when M8 begins rather than treating the September comparison as permanent.

### Coverage acceptance revision — 2026-09-09

The user accepts 80% statement coverage for Hand overall, changed production code and each critical subsystem. This supersedes the prior 90% changed-code and critical-group thresholds in the acceptance contract. All required scenarios, valid source/platform evidence, release integration, performance, review and authorised live-evaluation gates remain required. See `acceptance/definition-of-done.md` and `acceptance/check.py`.
