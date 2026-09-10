# Remaining-scope amendment — 9 September 2026

The user explicitly instructed: “can skip the remaining work listed, except execution isolation and harness release integration.” This supersedes the original all-M0–M8 completion objective for work still outstanding. Preserve delivered code and historical evidence; do not represent omitted work as passing.

## Required remaining work

1. **Execution isolation.** Finish and qualify the container boundary, explicit host-mode labelling, refusal to fall back to host execution, effective-boundary reporting and treatment of local/remote MCP and extensions. Verify filesystem, network, credentials and process boundaries with real controlled sentinel probes, including cancellation and cleanup. For the evaluation gateway boundary, establish that an isolated agent cannot obtain provider credentials or bypass the authorised route. Synthetic model responses may verify isolation; live model-quality comparisons are no longer required.
2. **Harness release integration.** Review and qualify the actual Harness changes, fix failures, prepare an exact release candidate and obtain any missing publication permission. Release the dependency, pin Hand to its published version/checksum, remove the local dependency override and verify Hand against that released dependency. Preserve migration/rollback documentation and run applicable regression, race and coverage checks. The user-approved 80% coverage threshold remains in effect for applicable qualification.

Tests, review and evidence necessary to establish these two outcomes remain required. Do not drop their negative-path or platform coverage because other work was removed. Publishing Hand itself remains unnecessary.

## No longer required as remaining deliverables

- Completing the Hand/Pi comparison executor and remaining agent-adapter breadth beyond what isolation tests need.
- Expanding the five-task calibration catalogue to thirty tasks or creating the held-out set.
- Authentic pricing/billing reconciliation and paid coding-quality comparisons.
- Unrelated outstanding full-plan performance, journey, platform and final acceptance work, except where needed to qualify isolation or the released Harness integration.
- Agent swarms remain outside scope.

The existing requirement manifest and full checker continue to describe the original scope and remain available as historical contracts. They must not be rewritten to claim omitted scenarios passed. A full-checker `not_complete` result for omitted requirements does not establish failure of this revised scope; conversely, completion of this revised scope does not establish completion of the original 221-scenario plan or superiority to Pi. Final reporting must name this amendment and report authentic evidence for both retained deliverables.

## Current access and evidence status

- Docker tests were explicitly authorised and executed. See harness-revised-qualified.json for candidate results.
- Harness publication was explicitly authorised on 2026-09-10. Release PR https://github.com/sausheong/harness/pull/4 merged after all checks passed; v0.4.0 is published.
- Hand now uses published Harness v0.4.0 without a workspace override. Native regression and isolated-agent gateway probes passed; final clean-candidate acceptance remains pending. Pi comparison evidence remains development-only and outside the retained work.
- Pi process-supervision work started before this amendment remains unqualified integration work; do not count it as complete or continue it unless needed for retained isolation testing.
