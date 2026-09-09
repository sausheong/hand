# Workspace file-reference completion

Tab now completes @file tokens asynchronously. Single matches replace the current token while preserving surrounding text and cursor position; multiple matches are listed. Spaced paths are quoted, directory paths retain a trailing slash, and incomplete quoted prefixes are accepted. Results are discarded after draft/cursor/workspace changes. Workspace-rooted directory scans read no file contents and enforce 4096-entry/64-match limits.

Tests cover quoted paths, nested directories, external symlink/traversal rejection, cursor ranges, match limits, cancellation, terminal insertion and stale-result rejection. Final focused checks passed 20 race-enabled repetitions. Full Hand validation passed 634 tests/subtests without failures/skips; vet passed. An initial incorrect expected byte offset in a test was corrected; its failure log is retained.

See [usage](file-reference-usage.md), [aggregate patch](file-completion-integration.patch), and [hashed evidence](file-completion-integration.json).

External policy, queued attachment resolution, native terminal/performance qualification and full acceptance remain pending. Integration remains staged against unpublished Harness; primary Hand uses v0.3.9.
