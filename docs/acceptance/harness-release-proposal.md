# Harness release prerequisite for Hand

Status: prepared locally; publication is not authorised and has not occurred.

> Historical proposal: the candidate and test counts below are superseded by [the current review snapshot](harness-current-release.md). Do not publish the old candidate.

## Reviewable candidate

- Repository: https://github.com/sausheong/harness
- Published main checked on 8 September 2026: `09c30fe2d529d6125f423593f621e970a7ce8735`.
- Local candidate: `308f702545c138fc5ad51a585c5d6f0a0f6ccc77`.
- Local checkout: `/private/tmp/hand-harness-development-20260908`, branch `codex/hand-hardening`.
- Reconstructable diff: `harness-development.patch`; SHA-256 in `harness-development.json`.
- Proposed release: **v0.4.0**, subject to confirming the tag is still unused. Latest published tag observed was v0.3.9. The minor version signals changed usage semantics in this pre-1.0 library.

## Resulting behaviour and migration

Context-aware runtime construction cancels MCP startup and cleans partial connections. Request-level usage includes failed/retried calls and compaction, while terminal usage remains cumulative. Anthropic InputTokens now includes cache counters; consumers must stop adding cache fields to InputTokens. Consumers of new request events must not double-count terminal totals. See candidate USAGE.md.

Subprocesses use Unix process-group cleanup, bounded inline output and private disk artifacts. Artifacts have per-file/store caps and lazy expiration; this is not containment of deliberately detached processes. Existing executable-file mode bits survive atomic replacement. Stdio MCP shutdown is bounded and idempotent.

## Validation and limits

The full local race-enabled suite passed 771 tests/subtests before freezing. Vet passed and all four darwin/linux arm64/amd64 targets built; these are build checks, not native execution on four targets. Go printed nonfatal sandbox module-cache write warnings during cross-compilation. Changed Go files pass formatting; unrelated pre-existing files have formatting differences. Fresh frozen-candidate test evidence is recorded separately in the manifest when complete.

The candidate includes `.github/workflows/qualify.yml`: native Linux/macOS build, vet, uncached race/coverage suites, repeated lifecycle regressions and raw evidence upload. Hosted execution is still pending. The workflow provides evidence for review; it does not itself publish a tag or establish Hand's full acceptance.

## Requested authorisation

Authorise pushing this candidate on `codex/hand-hardening` to the Harness repository, opening its review PR, running the included hosted checks, and merging/tagging v0.4.0 only after the native checks and release review pass. Recheck upstream refs and preserve concurrent work before merging. If fixes materially change the candidate, record the new commit and fresh evidence. Do not force-push main or replace a published tag.

This request is required by the active goal's instruction to obtain missing Harness release permissions. It does not authorise publishing Hand or spending on model evaluations. Without this permission, continue independent Hand work and keep released-version integration pending.

## Integration after release

Resolve the published v0.4.0 module and checksum without a local replace directive. Replace Hand's abandoned-goroutine MCP timeout wrapper with context-aware construction, propagate startup cancellation, adopt shared process cleanup for hooks, consume per-request events for the context gauge and update cache accounting together. Run real Hand regressions against the released version. Preserve previous release rollback instructions and document usage/output/timeout migrations.
