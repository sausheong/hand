# Coverage evidence and mapping

`scripts/validate.py` instruments all Hand production packages with `-coverpkg=./...`, so integration tests in another package contribute real executions. Profiles use atomic counters. Repeated coverage blocks from different test binaries are deduplicated, with their maximum execution count; statement percentages are weighted by instrumented statements, not averaged across packages.

`changed-coverage.json` derives changed lines from the recorded Git baseline, including untracked Go source in development runs. It counts a whole instrumented block when any added/changed line intersects it. Deleted statements do not appear in the candidate denominator. Missing changed source files in the profile are reported as gaps; they cannot produce a percentage-based pass. For absent files, the runner invokes Go’s coverage instrumenter and records source and instrumented-output hashes only when it proves zero counters. Such files contribute neither covered nor total statements. Parse failures, missing tools, unsafe paths and any executable counters remain gaps; platform build tags do not exempt executable source. This is statement coverage, not proof of branch or assertion quality.

`critical-coverage-map.json` records module-relative source patterns for nine required critical groups. Some groups intentionally overlap because shared lifecycle or accounting code serves multiple contracts; each group's blocks count only once. `pending_capabilities` names implementation gaps. Remove a pending item only when the associated capability exists, its source mapping is complete and the acceptance traceability review confirms it. Do not remove items or narrow source patterns to obtain a higher number. Future implementation files must be added to the mapping during their code review.

The critical reporter returns two distinct fields:

- `observed_percentage`: coverage of the mapped code present in the supplied profiles; this may be partial.
- `percentage`: a value only when source mappings resolve, required module profiles exist, statements exist and no capability is pending. Otherwise it is null and the group status is incomplete.

The ordinary Hand runner has no separately qualified Harness profile, so its affected groups remain incomplete. To inspect both modules during development:

```sh
python3 scripts/critical_coverage.py \
  --mapping docs/acceptance/critical-coverage-map.json \
  --profile github.com/sausheong/hand=/absolute/path/hand-coverage.out \
  --profile github.com/sausheong/harness=/absolute/path/harness-coverage.out \
  --output /absolute/path/critical-coverage.json
```

The standalone report hashes its input profiles and mapping. Hashes do not establish test origin or candidate compatibility. For final acceptance, the Harness profile must be produced from the exact released version used by the Hand candidate, with the required native/platform evidence. A development Hand profile using v0.3.9 and a new Harness candidate profile cannot establish integrated acceptance, even though their partial coverage can help locate missing tests.

No coverage report establishes feature completeness, meaningful assertions or platform correctness by itself. Final evidence must reconcile these reports with the requirement manifest and the acceptance review. Runner success is not a declaration that all percentage thresholds or the full plan have passed.

## Integrated development measurement, 9 September 2026

`primary-coverage.json` records the first cross-package measurement from the integrated primary checkout and its local Harness candidate. Hand measured 77.61% overall and 77.34% of observed changed statements. The changed-code report remains incomplete: native files for other platforms and Go fixture sources retained in evidence are absent from this macOS profile. These gaps require explicit source classification and platform evidence; they are not zero-statement passes. No source exclusions have been introduced.

The nine critical groups measured between 76.3% and 83.1% of their mapped statements, all below 90%. The mapping now includes the implemented RPC server, extension and package code, checkpoints, budgets, process backends and relevant Harness code. Three provisional patterns were corrected because those paths do not exist; `internal/rpc/*.go` supplies the actual RPC server path.

The full Hand run failed two runtime-probe tests and skipped twelve native/public-network cases. Harness passed its ordinary suite with five native skips. Follow-up diagnostics passed both affected Hand packages, but do not erase the full-run failures or establish their cause. Compressed raw profiles, JSON test logs and source snapshots are retained with both compressed and uncompressed hashes. This is development evidence; the final released candidate still needs fresh qualification.

A subsequent controlled full Hand run used `-p=1` to avoid simultaneous package workloads, retaining race detection and all existing timeouts. It passed 1,220 named test records across 26 packages, with 12 explicit native/public skips, and measured 77.66% coverage. This supports investigation of contention but does not prove the cause of the previous concurrent failures. Both runs remain in the report.

The validation runner now derives available coverage after failed or skipped tests and labels it `coverage_from_qualified_tests: false`, then exits unsuccessfully. Its regression tests verify that a 100% synthetic profile cannot override a failed test, skipped test or nonzero Go command exit. This preserves useful failure evidence without weakening any acceptance gate.

## Explicit test-driver scope and persisted-integrity tests

`coverage-scope.json` now enumerates seven Go test drivers that the changed-code reporter previously treated as production: six retained copies of the external SDK compatibility consumer and the MCP server under `internal/agentio/testdata`. Each entry records an exact path, SHA-256 and its test-only purpose. Classification was reviewed against the source comments and use as external test drivers. No wildcard exclusions, ordinary source paths or platform-source exclusions are permitted. A changed hash, missing file, symlink or duplicate entry fails classification; unlisted files remain in scope. The original example programs remain in the production denominator.

`integrity-coverage.json` retains new checkpoint and permission journal corruption tests, atomic profiles and source fingerprints. The tests assert rejection of invalid persisted state, preservation of corruption evidence and user files, absence of partial authority, and release of locks after failed replay. Both package suites passed without skips (87 checkpoint and 37 permission test/subtest records). The reporter's 17 regression tests also passed.

The combined development profile measures 77.83% Hand coverage. It combines maximum counts for unchanged production blocks from the controlled full run and the two new package runs; it is not a fresh final-candidate qualification. The checkpoint package alone measured 81.21%; its broader critical group includes application restore code and measures 80.39%. Every critical group remains below 90%. Six other-platform files still lack coverage in this macOS profile and remain explicit gaps. See the report for raw evidence and all limitations.

## Current coverage threshold — 2026-09-09

The user lowered changed-code and critical-group statement coverage requirements to 80%, matching the existing overall threshold. Earlier measurements and references to 90% above are historical. Missing platform profiles, source qualification and all non-coverage acceptance requirements still apply.

## Exact declaration-only paths in critical groups

The critical coverage reporter uses the same Go instrumenter proof as changed-code coverage when a module source root is supplied. The CLI accepts repeated `--source-root module=path` arguments; the validation runner supplies the Hand root. Only exact mapped files can receive zero-counter proofs, with source and instrumented-output hashes retained. Wildcards remain unresolved when they match no profile blocks. Missing module profiles, executable files and pending capabilities still prevent qualification, and a group containing no measured statements cannot pass. Source-root proofs do not establish profile freshness or candidate identity.
