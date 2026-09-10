# Explicit workspace file references

Added bounded @file snapshots to the shared terminal/one-shot input parser. Quoted and escaped paths preserve source text; repeated normalised paths deduplicate. UTF-8 text is JSON-encoded as labelled reference data in the user prompt. Errors reject the whole submission; external paths and symlink escapes remain denied.

Tests cover exact content encoding, snapshot stability, duplicates, quoted paths, missing/binary/oversized/external files, aggregate limits, cancellation and a TUI-to-backend reference journey. They passed 20 race-enabled repetitions. Full Hand validation passed 631 tests/subtests with zero failures/skips; vet passed.

See [usage](file-reference-usage.md), [aggregate patch](file-reference-integration.patch), and [hashed evidence](file-reference-integration.json).

External policy, queued references, path completion, standalone binary/native qualification and full acceptance remain pending. Reference framing is not a guarantee against all prompt injection. Integration remains staged against unpublished Harness; primary Hand uses v0.3.9.
