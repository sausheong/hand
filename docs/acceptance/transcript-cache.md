# Incremental transcript layout

The staged TUI now caches wrapped rows for each transcript block and maintains cumulative row offsets. Only changed blocks are wrapped again. A width change invalidates layouts; rendering finds the visible interval by binary search and visits only its rows. Clearing or replacing history discards obsolete cached content. Completed-history comparisons remain linear in block count; expensive text layout and painting do not.

Development benchmark on Apple M4 Max, macOS 26.6.2, Go 1.25.1 darwin/arm64, 80×24:

- 10,000 blocks; 30 runs × 34 measured events = 1,020 events.
- Aggregate p95: **0.175459 ms**; maximum: **0.4245 ms**.
- Earlier uncached implementation: p95 **21.029917 ms**, measured in one 34-event baseline run.

Each event includes a stream delta, transcript refresh, input key and View construction. Fixture creation, initial layout and physical terminal paint are excluded. These are development measurements, not exact-candidate native acceptance. Go's one-event calibration samples are retained in raw output but excluded from the reported 1,020 samples.

Reproduce in the integration checkout:

```sh
GOCACHE=/private/tmp/hand-review-gocache GOPROXY=off go test ./internal/tui -run '^$' -bench '^BenchmarkTranscript10000Blocks$' -benchtime=34x -count=30
```

The TUI race suite passed 210 test/subtest events; vet passed. Regression tests cover cache reuse, same-length content mutation, replacement, clearing, width changes, Unicode and visible-row selection. Evidence and aggregate source hashes are in `transcript-cache-integration.json`.

Other transcript content still uses rendered strings. Full typed-block migration, Markdown source reflow and text-delta coalescing remain pending, along with native qualification and released Harness integration.
