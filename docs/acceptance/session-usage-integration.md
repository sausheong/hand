# Staged persisted request usage

Hand's HarnessBackend now installs a usage observer before each runtime run. Each provider-attempt record is encoded as versioned `hand.request_usage` annotation data and durably written through Session.Annotate. Records retain request ID, model, category (generation/retry/compaction), outcome status, source and optional reported token fields. Missing usage is explicitly unavailable, including failed or cancelled attempts; it is not zero consumption. The observer retains the first persistence error and surfaces it through the backend after the runtime stream drains.

ReadUsage folds whole-session records across branches. Identical duplicate request IDs count once; conflicting IDs, unsupported versions, negative values, invalid cache subsets and integer overflow fail. Cached input is part of input, not an extra charge. A terminal backend accounting snapshot replaces the UI total after the run, avoiding double accumulation of cumulative Done usage. Startup and successful asynchronous resume restore the durable total and known/unknown attempt counts. `/usage` reports saved totals even before a new turn and discloses unknown attempts. Failed or cancelled records survive session reopening.

Session forks currently copy conversation history, not source accounting annotations: inherited context does not become a new billable attempt in the fork. Full raw JSONL export includes annotations. Explicit fork provenance, per-provider cost attribution and attachment portability remain separate unfinished work.

Permanent tests cover provider-reported, unavailable and failed attempts through the real Harness runtime adapter, durable reopening, duplicate/conflicting IDs, cache validation, future record versions, unchanged conversation leaf and resume/UI unknown-count restoration. These use deterministic provider fixtures, not live billing evidence.

Fresh staged full uncached race/coverage validation passed 560 tests/subtests with zero failures/skips. Usage tests passed 20 repetitions (140 passing events), and vet exited 0. Commands ran in `/private/tmp/hand-session-integration-20260908` with GOCACHE=/private/tmp/hand-review-gocache and GOPROXY=off:

```sh
go test -race -count=1 -json -coverprofile=/private/tmp/hand-session-usage-coverage-20260908.out ./...
go test -race -count=20 -json ./internal/sessionio ./internal/app ./internal/tui -run 'TestUsageSummary|TestUsageRejects|TestHarnessBackendPersists|TestResumeRestores'
go vet ./...
```

The aggregate patch and source/raw hashes are in `session-usage-integration.patch/json`. All modified staged Go sources were checked for inclusion. Earlier snapshots remain preserved. The patch excludes the temporary local module replacement. Primary Hand still uses released Harness v0.3.9 with this staged integration unapplied; development uses unpublished c16dccd. No full requirement group is closed.

Limitations: background-compaction shutdown/join and final accounting/error timing remain unfinished. The service suppresses display events after cancellation, so persisted cancelled-run usage may not immediately refresh the screen until a later reload/run. Historical sessions without usage records cannot reconstruct past consumption; explicit legacy-unknown presentation remains pending. Folding scans session annotations and needs scale qualification. Per-provider pricing/admission budgets, standalone manual-compaction accounting, attachments, native journeys, released dependency integration and the remaining plan gates are still required.
