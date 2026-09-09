# Reproducible evaluation inputs

`calibration.json` is a draft calibration task catalogue. It pins two real Harness defects and hashed verification fixtures. It deliberately has no agent or mode entries: no Hand/Pi revision, model configuration or spending authorisation has been invented. It is not the 30-task comparative evaluation and contains no held-out task.

Validate structure and fixture integrity:

```sh
python3 scripts/evaluation_manifest.py docs/acceptance/evaluation/calibration.json
```

Require qualification-sized inputs (the current catalogue correctly exits 1):

```sh
python3 scripts/evaluation_manifest.py docs/acceptance/evaluation/calibration.json --qualification
```

Exit 0 means the requested validation level passed, exit 1 means qualification inputs are incomplete, and exit 2 means malformed or inaccessible inputs. Always inspect the status. `qualification_inputs_valid` establishes input structure only; it does not prove task diversity, source availability, scoring quality, actual model snapshots, spend approval or evaluation completion. The validator never executes commands or model requests.

## Version 1 fields

- `schema_version`, `id`, `purpose`: versioned identity and calibration/qualification intent.
- `seed`, `order`, `repetitions`: recorded randomisation seed, counterbalanced execution order and independent paired repetitions. The future executor must implement the order and preserve each run's actual position.
- `per_run_budget`: finite timeout, request count, input/output token limits and maximum dollar cost. Zero dollars permits no paid calls. `planned_total_usd` must cover all declared per-run caps for qualification. These are planning limits, never an authorisation record.
- `agents`: unique IDs, source repository, full commit and explicit provider/model/reasoning fields. Qualification requires exactly Hand and Pi. The executor/review must resolve commits and model snapshots, record binaries and reject mismatches.
- `modes`: defaults and configured, each with per-agent tool lists, settings and packages. Package entries require identity and SHA-256. Store references to secrets separately; never embed credentials.
- `tasks`: unique ID, calibration/held-out split, category, exact prompt, independent code-review criterion, repository URL/full commit, relative working directory, hashed fixture copies and verification commands. Fixture sources must be regular files inside the manifest directory without symlink components. Source and destination paths must use canonical relative spelling; file destinations cannot be `.` or overlap another fixture destination. Git metadata paths are forbidden. The executor must enforce checkout containment and reject destination symlinks when copying; manifest validation alone does not establish runtime isolation.
- `verification`: nonempty explicit argv arrays, timeout and expected success exit 0. Commands remain data until a separately authorised executor runs them. The executor must inventory selected tests, reject empty selections/skips and preserve structured results and raw output.

Qualification validation requires 30 tasks, ten held-out tasks, three repetitions, both agents, both modes and all six planned task categories. This yields at least 360 runs. Before execution, review language/repository-size variety, isolate fresh workspaces, freeze held-out membership before tuning, resolve all source/package/model pins and obtain the concrete budget authorisation. No scheduling, cost accounting, task scoring or paid evaluator is implemented by this input validator.

## Calibrated file-mode oracle

The task uses Harness commit `09c30fe2d529d6125f423593f621e970a7ce8735`. In a fresh checkout, copy `testdata/write_mode_test.go` to `tools/file/acceptance_mode_test.go` after verifying its manifest digest, then run the declared verification command. The same test can check a candidate solution in a separate checkout.

Calibration against the pinned defective source failed with mode 0640 and 0751 becoming 0600. Calibration against prepared candidate `308f702545c138fc5ad51a585c5d6f0a0f6ccc77` passed. The test also checks replacement content and restrictive new-file permissions. It does not itself prove atomic-write correctness or rule out hard-coded solutions: the declared code-review criterion is required for task scoring.

Raw calibration logs and commit/command records are in `oracle-calibration/`. These prove the oracle distinguishes this known defect and fix; they are not Hand/Pi performance runs or live task-success evidence. The three validator tests include synthetic manifest dimensions solely to test validation rules.

## Paired schedule generation

`scripts/evaluation_schedule.py MANIFEST --out NEWFILE` generates a deterministic
execution plan only when qualification input validation passes. It does not run
an agent, spend money or grant approval. The current two-task calibration file
is deliberately rejected. The output is exclusively created to preserve prior
plans.

Each task/mode/repetition is one adjacent Hand/Pi pair. Agent-first order is
counterbalanced within each split and mode; pair order is shuffled with the
manifest seed. Calibration pairs precede held-out pairs. Every run contains its
exact task, agent settings, mode settings, budget and identities derived from the
complete canonical input manifest. Changing a model, prompt, budget or other
input changes run identities. The source file byte hash is also recorded.

A future executor must resolve the referenced source/model/binary pins, verify
fixtures, create an independent workspace/session for each run, enforce separately
authorised budgets and record actual execution order. Schedule order alone does
not provide held-out secrecy or independence. Freeze configuration before
held-out execution; if calibration changes inputs, produce a new manifest and
schedule before opening that phase. Preserve failed runs and explicit retries.

The scheduler's tests use a synthetic 30-entry input solely to verify dimensions,
counterbalancing and immutable run identities. Those duplicated test tasks are
not a real task corpus and are not model-performance evidence. The real 30-task
catalogue, repository/language-size diversity, calibrated oracles, executor,
scoring and authorised comparisons remain outstanding.

## Descriptive result reporting

`scripts/evaluation_results.py MANIFEST RESULTS --out NEWFILE` regenerates the
paired schedule from the frozen manifest and validates each record against its
run IDs. It neither executes runs nor grants permission to spend. The current
two-task calibration catalogue remains too small for this qualification reporter.

The result file has `schema_version: 1`, the scheduler's `manifest_sha256`, and a
`runs` list. Every run record contains:

- `run_id` from the regenerated schedule;
- `outcome`: `completed`, `failed`, `cancelled` or `timed_out`;
- `tests` and `review`: independently recorded `passed`, `failed`, `skipped` or `missing`;
- nonnegative integer `regressions` and `interventions`;
- `cost_usd`: a finite nonnegative amount, or null when cost is unknown;
- finite nonnegative `elapsed_seconds` for the measured attempt;
- boolean `recovery_attempted`, and `recovery_succeeded` as true, false or null;
- `evidence`: 1–64 records containing an exact relative `path` and `sha256`.

Artifact paths are relative to the result file's directory. Absolute paths,
traversal, symlink components, duplicate paths and digest mismatches are rejected.
The reporter preserves a failed/cancelled/timed-out attempt in the denominator.
Verified success requires completion, passed tests, passed review and zero
regressions. Missing runs yield null rates and paired comparisons instead of
being dropped. Unknown costs stay null in totals and cost per verified success;
known partial costs are reported separately. All recorded attempts, including
failures, contribute to cost per verified success. Reported costs above the
planned per-run cap remain visible in `cost_cap_violations`; this check is not
request/token/time enforcement or spending authorisation.

Default and configured modes have separate summaries. Each mode reports all,
calibration and held-out splits, including verification gaps, outcomes,
interventions, recovery and median elapsed time. Paired success differences are
Hand minus Pi. A deterministic 2,000-replicate percentile bootstrap resamples
whole task clusters, preserving the paired agents and repeated runs inside each
task. Its 95% interval is conditional on this selected task set; repetitions are
not treated as independent new tasks. It does not establish general superiority,
and no cost or latency confidence interval is currently produced.

Even a structurally complete set returns `records_complete_unreviewed`.
Artifact hashes alone cannot establish that a test/review outcome is truthful,
that binaries/models match their pins, or that spending was authorised. A future
executor and independent acceptance review must reconcile these fields with the
actual commands, provider accounting, outcomes and review evidence. Files should
be frozen before reporting; input hashes identify the bytes parsed by the CLI.
The output is created exclusively, preserving existing reports.

The reporter tests use synthetic records and a repeated structural task fixture.
They are validation tests only, not a real corpus, model-performance evidence or
proof that the full evaluation requirement is complete. Real task preparation,
source/model freezing, authorised execution, scoring reconciliation and final
comparison remain outstanding.

## Calibrated shell-cancellation oracle

`cancel-shell-descendants` uses the same defective Harness commit as the file-mode
task and a separate public-API oracle. It starts a shell descendant that signals
readiness, cancels the tool context, requires the command to join within one
second, and observes beyond the child's scheduled write. This prevents early
parent exit alone from satisfying the test. The fixture joins the command on
failure so its temporary workspace is not removed while the known child runs.

The defective revision failed both the join deadline and the no-late-write
assertion. Candidate `308f702545c138fc5ad51a585c5d6f0a0f6ccc77` passed the same
fixture. The code-review criterion still requires general process ownership and
protection of unrelated processes; passing this one oracle does not prove every
subprocess lifecycle case. Raw pinned-source archive identities and both logs
are recorded in `../evaluation-calibration-cancel.json`. An initial command
issued from the wrong directory is retained as setup failure, not bug evidence.

The catalogue now has two Go bug-fix calibration tasks. It still needs at least
28 additional real tasks, category/language/repository-size diversity, ten or
more held-out tasks, and frozen agent/model configurations. Neither calibration
run is a Hand/Pi agent evaluation or a paid model request.

Result-reporter CLI exit codes are 0 for complete-but-unreviewed records, 1 for
incomplete records (with a report written), and 2 for invalid/inaccessible input
or an existing output file. A zero exit code does not certify qualification.
CLI regression tests exercise input hashes, incomplete output, invalid-input
rejection and preservation of prior reports.

## Verify local source pins and inventory diversity

`scripts/evaluation_sources.py MANIFEST REPOSITORY_MAPPINGS --out NEWFILE`
checks source commits using explicitly selected local repositories. The mappings
file is a JSON object mapping each declared repository URL to an absolute local
repository path. The tool does not fetch, check out a worktree, run hooks or
execute source code. It ignores dirty/untracked files and clears Git environment
overrides that could redirect the mapped repository.

Each unique URL/commit has its commit and tree identity, raw Git inventory hash,
file object IDs, file modes, byte sizes and language-extension counts recorded.
Regular files are counted separately from symlinks and submodules; targets are
not followed. Counts include tests, vendored and generated files and should not
be presented as authored-code line counts. A local mapping and available Git
objects do not prove remote repository ownership or qualify a task oracle.

Missing mappings produce an incomplete report and exit 1. Invalid mappings,
noncommit pins and inaccessible objects exit 2. Exit 0 means local source pins
were available, even if the catalogue remains a draft. Output files are created
exclusively. Source/fixture validation does not authorise model runs or spending.

The current two-task catalogue resolves to one Harness commit with 162 regular
files and 1,126,234 regular-file bytes, including 151 Go files. This inventory
makes the remaining repository/language diversity gap visible; it does not
satisfy it. Evidence is recorded in `../evaluation-source-verification.json`.
