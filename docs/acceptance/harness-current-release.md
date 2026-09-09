# Current Harness review snapshot

This is a development release-review bundle for the remaining scope in the [user amendment](scope-amendment-2026-09-09.md). No release candidate commit has been frozen or published.

The [current cumulative patch](development-evidence/harness-isolation-review-20260909/harness-current.patch) applies to historical base `09c30fe2d529d6125f423593f621e970a7ce8735`. It reconstructs all 302 files of the current Harness working tree, including the CI test-event qualification gate. Do not stack older patches onto it.

- Source checkout HEAD: `5998569d24ce063207cc237263d89f22d532e25f`; dirty changes are included in the patch, not that commit.
- Source fingerprint: `74155418fff918484a73b42f19261fb7975f67a08c42f2e80906edc2e6a923b2`.
- Patch SHA-256: `74a91bee53444ac700e1526e533533af6a5b7972b1be0d4cc45c89e0a0ee27cc`.
- Reconstruction evidence: [harness-isolation-review.json](harness-isolation-review.json).

Indexed apply checking and actual application to an independent checkout passed. Every file byte, symlink and executable mode matched the source. The original source tree and index remained unchanged. `go vet ./...` passed with `GOWORK=off` and `GOPROXY=off`.

The qualification event gate has five passing regression tests and rejects skipped/failed/unfinished streams. This is gate-tool evidence, not native Harness qualification; see [harness-qualification-gate.json](harness-qualification-gate.json). Hosted workflows still require configured container fixtures and now fail when those tests skip.

Earlier full native suites predate this fingerprint and cannot qualify this patch. The current Docker qualification suite has not started: automatic review rejected privileged socket access, and specific user permission is pending. See [approval record](harness-refresh-approval.json). No indirect or bypass execution has occurred.

Before final publication approval: complete exact-source native/race/coverage and isolation suites, address failures, review cumulative source and upstream changes, and freeze the release candidate. The proposed version is v0.4.0; the latest public tag lookup found no matching tag, but availability must be rechecked immediately before tagging. No push, merge, tag or publication was performed.

After authorised publication, pin Hand to the published module and checksum, remove the local workspace override and collect fresh applicable Hand regression/isolation evidence against the released dependency. The retained work is execution isolation and Harness release integration; unrelated full-plan evaluation and qualification work was removed by the user amendment.

## Pending native-oracle additions

`execution/backend_test.go` now requires network probe utilities and adds a child-write cancellation oracle. The execution test binary compiles, but these tests have not run because Docker access remains pending. The current cumulative patch now includes these tests and reconstructs all 302 files exactly; independent go vet passes. See [isolation oracle status](isolation-oracles.json).

## Docker authorised and current suite completed

User explicitly authorised Docker use. The first run found a test-only ip PATH mismatch; it is retained in `harness-authorised-first.json`. After explicit /sbin/ip lookup, the full uncached race/coverage suite passes all 1114 test records across 32 packages, no skips/failures/missing tests, unchanged source. See [current native evidence](harness-authorised-corrected.json) and [20 cancellation trials](authorised-cancellation.json). This supersedes earlier access-pending statements. The test correction changes source beyond the cumulative patch above; refresh that patch before release. Independent Linux platform qualification, exact candidate review/publication and Hand released-dependency integration remain pending.
