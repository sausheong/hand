# Reproducible evaluation inputs

`calibration.json` is a draft calibration task catalogue. It pins two real Harness defects, one real Hand search defect and one Kokoro-ONNX configuration defect and one GoClaw explainable-routing feature, with hashed verification fixtures. It deliberately has no agent or mode entries: no Hand/Pi revision, model configuration or spending authorisation has been invented. It is not the 30-task comparative evaluation and contains no held-out task.

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
- `verification`: nonempty explicit argv arrays, timeout, expected success exit 0, `result_format` and nonempty `expected_tests` identities. Supported formats are `go_test_json` (explicit `go test -json -count=1`) and `python_unittest_json` (the hashed unittest driver described below). Both require exact package/test identities. Commands remain data until a separately authorised executor runs them.

Qualification validation requires 30 tasks, ten held-out tasks, three repetitions, both agents, both modes and all six planned task categories. This yields at least 360 runs. Before execution, review language/repository-size variety, isolate fresh workspaces, freeze held-out membership before tuning, resolve all source/package/model pins and obtain the concrete budget authorisation. No scheduling, cost accounting, task scoring or paid evaluator is implemented by this input validator.

## Calibrated file-mode oracle

The task uses Harness commit `09c30fe2d529d6125f423593f621e970a7ce8735`. In a fresh checkout, copy `testdata/write_mode_test.go` to `tools/file/acceptance_mode_test.go` after verifying its manifest digest, then run the declared verification command. The same test can check a candidate solution in a separate checkout.

Calibration against the pinned defective source failed with mode 0640 and 0751 becoming 0600. Calibration against prepared candidate `308f702545c138fc5ad51a585c5d6f0a0f6ccc77` passed. The test also checks replacement content and restrictive new-file permissions. It does not itself prove atomic-write correctness or rule out hard-coded solutions: the declared code-review criterion is required for task scoring.

Raw calibration logs and commit/command records are in `oracle-calibration/`. These prove the oracle distinguishes this known defect and fix; they are not Hand/Pi performance runs or live task-success evidence. The three validator tests include synthetic manifest dimensions solely to test validation rules.

## Paired schedule generation

### Integrated offline baseline verification

`scripts/evaluation_baseline.py` connects input validation, pinned checkout
preparation, fixture installation, bounded commands and exact test-inventory
validation. For a reviewed offline task:

```sh
python3 scripts/evaluation_baseline.py \
  /absolute/path/to/evaluation/calibration.json \
  --task preserve-write-mode --repository /absolute/path/to/local-harness \
  --output /absolute/path/to/new-evidence-directory \
  --environment /absolute/path/to/offline-environment.json
```

The environment file is an explicit string mapping for executable search and
local tool caches. Do not include model credentials. Output must be a new
directory: it contains the independent workspace, separate logs for each check,
and `report.json` with the input-manifest hash, source and fixture records,
commands and verification results. The environment's key names are recorded;
secret values are not copied into the report. Preparation/execution exceptions
produce `baseline_error` evidence when the output directory has been created.
Validation errors before preparation leave no output directory.

Exit 0 means all declared baseline tests passed; exit 1 means baseline tests
failed. Preparation or execution exceptions also exit nonzero. The current
catalogue tasks target defects or absent features, so their expected CLI
result is `baseline_tests_failed` with the exact oracle present and failing.
That calibrates a baseline; it is not a failed agent run. Existing evidence is
never overwritten, and retrying requires a new directory.

This command does not execute agents or grant spending authority. It runs local
verification code with host access, so its present use is limited to reviewed
offline baseline commands. The comparative executor still needs agent isolation,
request/token/cost enforcement, model/source pins, review and scoring. No
baseline result can establish that Hand is better than Pi.

### Verification fixture installation

First create a new independent repository from an explicitly mapped local source:

```sh
python3 scripts/evaluation_checkout.py \
  --repository /absolute/path/to/local-source \
  --commit FULL_TASK_COMMIT \
  --destination /absolute/path/to/new-checkout
```

The destination must not exist. Preparation copies Git objects through a local
fetch without object alternates, binds HEAD and the index to the pinned commit,
and writes the exact committed blobs and executable modes. Dirty and untracked
source files, checkout filters, global Git configuration, templates, and
export-ignore/substitution do not change these input bytes. Source inventory
commands prohibit transport and lazy fetching; only the explicit preparation
fetch permits local file transport. Output includes the commit, tree and each
file's SHA-256. No repository build or verification command runs during this step.

Preparation currently rejects trees containing symlinks or submodules rather
than omitting them or following their targets. It also rejects sources exceeding
1 GiB or individual blobs exceeding 64 MiB. These are explicit preparation
limitations to resolve when selecting the full task corpus. The parent directory
must be exclusively owned during preparation. Failed preparation invalidates
the destination; preserve its error evidence and use a new destination.

After preparing a fresh, independently owned checkout, install the selected
task's pinned verification fixtures with:

```sh
python3 scripts/evaluation_workspace.py \
  /absolute/path/to/evaluation/calibration.json \
  --task preserve-write-mode --workspace /absolute/path/to/fresh-checkout
```

The installer verifies all source hashes before writing and checks existing
destination paths before creating files. It opens every root and relative path
component without following symlinks, rejects Git metadata and overlapping
destinations, and exclusively creates private files. Existing files are never
overwritten. Limits are 128 fixtures, 16 MiB per file and 64 MiB in total; exceeding
them fails preparation instead of truncating a fixture. OS path aliases such as
`/tmp` must be supplied using their resolved path, for example `/private/tmp`.

The checkout must be idle and exclusively owned during preparation. These file
operations are not a sandbox against a concurrent host process renaming checkout
directories. Any installation error invalidates the workspace: discard that
fresh checkout, retain the error evidence, and prepare a new one. An I/O failure
can leave partial files, and the installer deliberately refuses an overwrite on
retry. Successful output records exact fixture digests and byte counts.

The fixture installer only copies verification files. It does not check out source,
execute verification, run agents, score tasks or authorise spending. The future
executor must place fixture installation in the verification phase, preserve
independent clean workspaces, and prevent the agent from changing the oracle.

`scripts/evaluation_schedule.py MANIFEST --out NEWFILE` generates a deterministic
execution plan only when qualification input validation passes. It does not run
an agent, spend money or grant approval. The current five-task calibration file
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

### Bounded command primitive

`scripts/evaluation_process.py` provides `run_command(argv, cwd, evidence,
timeout_seconds, max_output_bytes, environment, cancelled=None)` for the
executor. It accepts explicit arguments without a shell and an explicit
environment without inheriting credentials. It creates a new POSIX process
group, records stdout and stderr separately outside the checkout, and enforces
a combined output-byte limit and monotonic timeout. An optional cancellation
event stops a running command; cancellation before launch raises
`InterruptedError` without starting a process.

Results distinguish `completed`, `failed`, `timed_out`, `cancelled`,
`output_limited` and `cleanup_failed`, retaining the exit code, measured duration,
raw log hashes and whether the direct process and output pipes joined. The
runner signals its owned process group on normal exit as well as interruption,
so ordinary descendants do not outlive a completed command. This is not a
sandbox against a malicious process escaping with `setsid()`, and joined pipes
do not prove that every descendant PID has disappeared. Evidence directories
are exclusively created; errors never become success records.

This primitive does not grant execution or spending authorisation, interpret
test output, detect skipped tests, or enforce model request/token/cost budgets.
The comparative executor must add those controls and use the required isolation
boundary before running agents or untrusted verification commands. Development
calibration used explicit offline Go commands and no model credentials.

### Go verification events

`scripts/evaluation_go_tests.py` provides `verify_go_tests(output, execution,
expected)` for commands explicitly run with `go test -json`. `expected` is a
nonempty list of exact `package` and `test` identities. Qualification must freeze
this inventory with the task; it must not be inferred from whichever tests happen
to pass. The command, including `-json`, must also be frozen before comparison.

Verification requires a completed command with exit code zero and joined output,
every expected test to run and pass, valid package/test event transitions and
terminal results, and no failed or skipped test/package. Empty selections,
truncated streams, duplicate terminal results, ambiguous duplicate JSON fields
and malformed events fail validation. Parallel test pause/continue events and
subtests are supported. Repeated tests in one invocation require a future
explicit attempt format; duplicate identities are currently rejected.

Development calibration ran each real oracle with `-json` against both known
defective and fixed commits, correctly producing two failures and two passes.
A real Go invocation selecting no oracle returned zero but failed this validator.
These establish verifier behaviour, not agent task success. The parser cannot
authenticate output or protect the oracle from an agent; isolated execution,
fixture integrity and independent review remain necessary. The Python unittest format is described below. Other language formats and
the comparative executor integration remain outstanding.

`scripts/evaluation_results.py MANIFEST RESULTS --out NEWFILE` regenerates the
paired schedule from the frozen manifest and validates each record against its
run IDs. It neither executes runs nor grants permission to spend. The current
four-task calibration catalogue remains too small for this qualification reporter.

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

## Verify a reviewed candidate patch offline

Run `python3 scripts/evaluation_baseline.py MANIFEST --task TASK --repository LOCAL_REPOSITORY --output NEW_ABSOLUTE_DIRECTORY --environment ENVIRONMENT_JSON --candidate-patch PATCH`. Omit the patch option to verify the baseline. Use explicit `PATH`, `GOCACHE`, `GOMODCACHE`, `GOPROXY=off`, `GOWORK=off` and `GOTOOLCHAIN=local` values for the Go calibration tasks. Never include model credentials.

The runner prepares the pinned baseline, archives the exact patch and its SHA-256, checks and applies it with Git's index, records the resulting tree, then installs the pinned acceptance fixtures. A patch that occupies a fixture destination is refused; introduced symlinks/submodules are unsupported and refused before test execution. Fixtures are not part of the recorded candidate tree. The source checkout is unchanged.

Results are `candidate_tests_passed`, `candidate_tests_failed`, or `candidate_error`; errors retain the patch and report after output preparation. Passing tests exit zero but do not certify task success. Review the actual diff, regression scope and output authenticity independently. This executes candidate code locally and is only for reviewed offline inputs; it does not provide agent isolation or paid-evaluation authorisation.

Both real Harness calibration tasks passed with the known historical fixing patch. The initial missing-module-cache failures and corrected runs are preserved in `../evaluation-candidate.json`. All 97 Python runner tests passed, including broken candidate, invalid patch and fixture replacement cases.

After each command, the runner checks every pinned acceptance fixture with bounded, descriptor-relative reads that refuse symlinks and non-regular files. Missing or changed fixtures make the overall result fail even if the test process exits zero, and later checks are not run. Each check records observed hashes and integrity errors separately from test events. This catches persistent mutation only: code that restores an altered fixture before observation, forges events, or tampers with the host process still requires isolation and independent review. Evidence and the reproduced false-pass regression are in `../evaluation-fixture-integrity.json`.

## Large-file Hand search calibration

`search-large-text` pins Hand `e6dd26351c1b1436616e1c2b8da0086a3580d882`. Its hashed oracle checks matches beyond 64 KiB at two different lengths, exact line numbers, filename filtering and a one-result cap. The baseline fails with missing matches; a minimal patch removing the size exclusion while retaining streaming scanning passes. Raw evidence is indexed by `../evaluation-search-calibration.json`. This adds a second source repository but no language diversity or held-out task; all three tasks remain calibration inputs, not live agent evaluations.

## Python unittest verification

Copy `scripts/evaluation_unittest.py` into the manifest fixture directory, record
its SHA-256, and install it as a verification fixture alongside the oracle.
Use `result_format: "python_unittest_json"` with explicit arguments such as:

```json
["python3", "-I", "oracle_driver.py", "--start-directory", ".", "--pattern", "test_acceptance*.py"]
```

The driver path must be a canonical relative path matching a hashed fixture
installed relative to the task's working directory. Expected identities use
`package: "python.unittest"` and the full unittest ID, for example
`test_acceptance.Oracle.test_preserves_value`. Freeze that inventory before
execution; do not derive it from passing output.

The driver emits the same strict start/run/terminal event schema as the Go
verifier, with the explicit Python package identity. Ordinary test stdout goes
to stderr, preserving JSONL output. Failed subtests and cleanup, import/class
setup errors, skips, expected failures, unexpected successes and empty discovery
cannot pass. The shared event validator also rejects missing expected tests,
malformed streams and incomplete command cleanup. Baseline verification installs
and checks fixture hashes before accepting the result, just as for Go.

Python `-I` isolates interpreter configuration; it does not sandbox test code.
Use reviewed offline commands here and the required isolation boundary for agent
comparisons. Tests of this adapter use small synthetic Python repositories and
assert before/after behaviour. They are tooling evidence only and add no task
to the real comparison corpus. Python dependencies, source pins, real oracles,
the execution budget and independent result review remain required.

## Kokoro-ONNX configuration oracle

`kokoro-config-reject-directories` pins public source commit
`8534fd94db73be4f3d07b3eefd623c1eb8c8a806`. Its configuration validator checks
existence but accepts directories as model/voices files. The Python oracle loads
only the configuration module: no ONNX runtime, model weights, audio processing,
credentials or network calls are needed. It tests both directory rejections,
both existing missing-file download hints, and regular-file/symlink acceptance
without configuration mutation.

Offline calibration reports two failures and three passes on the pinned source,
then five passes with a minimal path-validation patch in a fresh checkout.
Fixture hashes remain intact in both runs. See `../kokoro-oracle.json` for raw
logs, commands, source tree inventory and the calibration patch. This validates
one real Python task's oracle; it is not an agent solution or comparative result.
The catalogue now contains four calibration tasks from three repositories in
Go and Python, with no held-out tasks. The 30-task qualification gate remains open.

## Credential-free Pi runtime probe

`scripts/check_pi_offline.py --runtime RUNTIME_PREFIX --output NEW_DIRECTORY`
expects an isolated npm prefix containing Pi 0.85.1 and a compatible Node binary
on PATH. It verifies package identity, installs the reviewed deterministic
provider fixture temporarily, and exercises success, retry and abort in separate
fresh sessions. Explicit extensions disable discovery; no tools, skills or
prompt templates are enabled. The fixture makes no network/model calls.
The output directory must be new; raw JSONL, stderr, runtime/fixture hashes and
the report are retained. Every case must settle, return correlated statistics
and exit zero. The cancellation outcome must still be `cancelled`.

The executed probe used the package checked against registry SRI and the public
release commit recorded in `../pi-runtime.json`. Its three actual output streams
are compressed fixtures under `testdata/pi-rpc-v0.85.1/`. Replaying those streams
tests observer compatibility; it does not rerun Pi. Source-to-package attestation,
real provider traffic, budget enforcement, tool/process isolation and comparative
task scoring remain separate unfinished work.

## GoClaw explainable routing feature

The portable source pin and before/after calibration are recorded in `../goclaw-oracle.json`. The baseline lacks `Router.Explain` and fails compilation; this is distinct from the initial nonportable pin whose dependency setup failed. A developer-written calibration patch passes six tests under race detection, including all 24 priority permutations, combined constraint rejection, fallback, stable first-match ties and concurrent reads. This adds a feature category to the five-task, four-repository catalogue; all tasks remain calibration inputs, with no held-out or real-model results.
