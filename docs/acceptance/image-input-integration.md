# Explicit image parsing and errors

Added a strict image-input parser for normal TUI submissions. Quoted paths, escaped spaces and optional @ prefixes preserve source positions; repeated normalised paths attach once. Reads use a workspace root and bounded file/prompt/image-count limits. Errors return the original text with no partial image set. The terminal reports the attachment error and retains input without starting a goal.

Tests cover spaced paths, duplicate references, apostrophes in prose, missing/oversized/external/symlink images, cancellation, aggregate byte limits and TUI draft preservation. They passed 20 race-enabled repetitions. Full Hand validation passed 623 tests/subtests without failures/skips; vet passed.

See [usage](image-input-usage.md), [aggregate patch](image-input-integration.patch), and [hashed evidence](image-input-integration.json).

External attachment policy, content-based validation, queued images, CLI parity, generic file references/completion and final qualification remain pending. The legacy non-error extraction helper remains for existing internal callers/tests; the normal terminal uses the strict parser. This remains staged against unpublished Harness; primary Hand uses v0.3.9.
