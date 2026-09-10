# Publication result

Harness v0.4.0 is published at https://github.com/sausheong/harness/releases/tag/v0.4.0 . PR #4 merged after all four hosted checks passed. The immutable tag targets tested commit `5d4632ffe3fdfeb171bb217a45ae75bef056811e`; main merge commit `4cec5b57f874a5fb074459b31e75ad4050617496` has the same source tree. Each platform passed 1,114 test records and 80 lifecycle repetition records without skipped tests. See harness-publication-status.json for raw evidence. The review below preserves the preparation history.

# Harness v0.4.0 release review

Status: publication authorised; release PR https://github.com/sausheong/harness/pull/4 is qualifying. The current release head is `a1c4d5d78c2e3a6832cfe5e48476a099936c6dd1`, which adds hosted macOS Docker provisioning and stopped-container fixture creation. Hosted Linux passed 1,114 records and 80 repeated lifecycle records. Hosted macOS is pending. Earlier local evidence below remains tied to its original commit.

Candidate: `cc756425e8b0c2366b08d33f3184cf9ede3e6f7e` on local branch `codex/hand-release-v0.4.0`. Repository: `https://github.com/sausheong/harness`. The candidate descends from public main `09c30fe2d529d6125f423593f621e970a7ce8735`; recheck public refs immediately before any remote write. Proposed tag `v0.4.0` was absent at the latest lookup. Never replace an existing tag or force-push main.

The [candidate record and Git bundle](harness-frozen-candidate.json) identify the exact source. The original development checkout is preserved. The release changes runtime cancellation/producer ownership, bounded process output and cleanup, persistent session graphs/migration/recovery, request-level usage and budgets, and explicit container execution. CI now prepares and records Docker fixtures and rejects skipped or incomplete tests.

## Required consumer migration

- Anthropic `Usage.InputTokens` now includes cache-read and cache-creation input. Do not add those subsets again. For 10 uncached, 42 cache-creation and 17 cache-read tokens, total input is 69. Consumers of request events must also avoid double-counting terminal aggregates.
- Unknown request usage remains unknown, including potentially billable errors. It must not be treated as zero cost. Producers and streams must be joined before releasing their session or accounting owner.
- New session files have a version-1 identity header. Legacy files are readable without mutation; explicit migration preserves an original backup. Raw exports retain the header. Unsupported versions and complete corrupt records fail rather than being silently repaired.
- Container execution is explicit. Missing runtime support must not fall back to host execution. Host-side integrations and provider requests are distinct from the tool container boundary.

Authoritative consumer contracts are `USAGE.md`, `SESSION_FORMAT.md` and `attachment/README.md` in the candidate bundle.

## Verification and current limits

The prior candidate `0972eed3eabe53dd310239bc66643aa89b915bd0` passed 1,114 test records on both macOS and Linux and 20 real-container child-cancellation repetitions. Those records remain historical for that exact commit. The current revision adds CI fixture preparation and documentation; fresh full platform runs passed 1,114 test records each with no failures or skips; its 20 cancellation trials also passed. See [exact-candidate evidence](harness-revised-qualified.json).

The actual CI fixture shell step ran locally, exported its exact image IDs, and passed all 45 execution/bash fixture test records. Workflow YAML parsing and shell syntax validation passed. This is not a hosted GitHub workflow result.

The current workflow provisions Colima on an ephemeral hosted `macos-15-intel` runner. Docker fixture setup succeeded on both release jobs. No developer machine was registered as a self-hosted runner; skipped tests still fail qualification.

## Publication and Hand integration sequence

1. Finish the current exact-candidate native suites and inspect their raw failures, coverage, source identity and cleanup results. Resolve actual release-blocking findings before seeking publication approval.
2. Obtain missing permission for the exact public branch/PR and release actions. Hosted runner configuration is a separate access prerequisite; do not fabricate hosted checks or bypass branch protection.
3. Recheck upstream refs. Push the reviewed candidate without overwriting concurrent work; merge/tag only under the authorised release procedure. Record the public commit and immutable tag target.
4. Resolve `github.com/sausheong/harness@v0.4.0` from its published source, verify module identity/checksum, pin Hand's go.mod/go.sum and remove the local workspace override. Never substitute a local replace directive for the released dependency.
5. Run the retained Hand isolation/regression checks against the released module, including applicable 80% coverage gates. Record source/worker identities and raw evidence.

Before upgrading persisted state, retain the original synced session/attachment backups and prior Hand binary/configuration. A downgrade must restore compatible pre-migration state from those backups; do not assume v0.3.9 can safely read newly rewritten formats. Public Hand publication is outside the required scope.
