# Retained-scope acceptance review

The human scope amendment of 9 September retains execution isolation and Harness
release integration. This review does not mark omitted original-plan scenarios
as passing. Implementation and development qualification have progressed; final
clean-candidate acceptance remains outstanding.

## Isolation: original requirement M5.2

| Scenario | Implementation and regression evidence |
| --- | --- |
| mount_read_write_escape_denied | Harness TestNativeContainerBoundary checks symlink escape, read-only mounts, root writes and an explicit writable-workspace positive control; Hand TestWorkspacePathMappingPreservesContentAndRejectsEscape and TestNativeProxyWorkerRoundtrip cover proxy routing. |
| network_disabled | Harness tests require working network-probe utilities and no routes; the isolated-agent probe also verifies a live network sentinel before denying access from network=none. |
| credentials_scoped | Harness tests prove explicit environment allowlisting and absent inherited secrets. The isolated-agent probe keeps the provider credential and budget store in the separate gateway namespace. |
| children_inherit | Harness child filesystem probes and TestNativeCancellationStopsChildWrites; Hand TestNativeBackgroundContainerInputOutputAndShutdown verifies background input/output and joined shutdown. |
| missing_backend_no_fallback | TestMissingContainerNeverExecutesHostCommand, TestMissingBackendIsNotHostFallback, TestExecutionConfigurationFailsClosed, and atomic rejection of unsupported tools. |
| mcp_extension_boundary_visible | Explicit MCP/hook external-trust validation and boundary strings; container extension activation/reload/resource tests and SDK boundary journeys. Host integrations remain labelled outside the tool container. |
| linux_macos_container_suite | Harness v0.4.0 passed hosted Linux and Intel macOS suites. Hand passed native macOS and Linux ARM64-in-Docker suites against that published module. The latter uses host-shared workspaces, not an independent physical Linux host. |

The new gateway probe exercises a real Hand tool turn through the existing
admission controller, with negative authentication/model/route probes, credential
and filesystem sentinels, actual network denial and verified resource cleanup.
See evaluation/isolation.md and evaluation-isolation.json.

## Harness release integration

Harness v0.4.0 is published from commit
5d4632ffe3fdfeb171bb217a45ae75bef056811e. PR #4 merged only after all four hosted
checks passed. The module checksum is
h1:HoGrAbqYvmgsuR3RU14f6hHrOogrDhmSjqzQWsbEhMg=.
Hand pins that published version and no longer has a local workspace override.
Migration/rollback instructions are retained in the release notes and release
review. A startup EOF race found in integration was fixed and verified with a
forced-ordering test, 20 binary repetitions and the full native suite.

## Coverage reconciliation

Pre-fallback-refactor evidence contains 1,910 macOS and 1,912 Linux passing test
records, with no failed or skipped records. Identical Go-source hashes permit
combining their profiles: 82.02% overall and 81.81% observed changed coverage.
Two standalone evaluation Go drivers were explicitly fingerprinted as test
fixtures under the existing coverage-source policy; no production code was
excluded.

The six remaining platform-only gaps now contain declaration-only selectors.
Their executable fail-closed implementations moved to common files, and tests
assert rejection without acquiring authority or changing real sentinel files.
Go's instrumenter independently proves the selectors have zero counters. The
selected fallback files cross-compile, but whole-project Windows compilation
still fails on unrelated Unix-specific APIs. No Windows support is claimed.
The common executable bodies remain in the coverage denominator and require
fresh final-candidate profiles.

## Remaining completion gates

1. Review and freeze a clean Hand candidate containing the final runners,
   portable fallbacks, tests and scope documentation; preserve the original
   working checkout and do not publish Hand.
2. Collect fresh native macOS and Linux qualification for that candidate with
   Harness v0.4.0, full race suites, no skipped/incomplete tests, and worker hashes.
3. Rerun the isolated-agent probe using that candidate's Linux binary and exact
   gateway source. Verify owned-resource cleanup and immutable input hashes.
4. Reconcile overall, changed and every applicable critical-group coverage at
   the user-approved 80% threshold, with no missing executable source evidence.
5. Complete a machine-checkable retained-scope audit tied to raw evidence and
   candidate identity. Run and retain the original full-checker output without
   relabelling omitted work as passed. Report revised-scope completion separately
   from the original 221-scenario contract and Pi comparison claims.

## Final audit command

Run `python3 scripts/check_remaining_scope.py --evidence /absolute/path/run.json`
from the clean candidate. The evidence bundle must contain hashed raw platform
reports, inventories, test streams and coverage; published Harness qualification
artifacts; the isolated-agent report; the final review; and the original checker
output. The checker recomputes metrics and verifies source/dependency identity.
A `complete` result applies only to the explicit user amendment. The original
full-plan checker remains unchanged and its `not_complete` result stays visible.
