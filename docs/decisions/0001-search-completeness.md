# ADR 0001: bounded Go search with explicit scope

Status: accepted implementation choice, 8 September 2026. Covers M1.2 and contributes one benchmark fixture to M0.2.

Use rooted Go filesystem access, streaming regex scanning and go-git's gitignore matcher (pinned v5.16.2). No executable Git or rg is needed. The parser was already available in the module cache; its documented domain/priority matching API supports nested patterns. The dependency adds several transitive module requirements; reassess upgrades through normal dependency review, not an implicit latest-version assumption.

Load workspace and nested .gitignore files with increasing priority and bounded total bytes/patterns. A directory excluded by its parent is not traversed merely because a descendant pattern attempts to re-include a child. Explicit include_ignored/include_hidden controls broaden the search scope; .git metadata always remains excluded. Host-global Git exclusions are deliberately not loaded because results should be reproducible from workspace inputs.

Return readable matches, a completeness footer and structured metadata. Complete means exhaustive within the stated scope, not that ignored, hidden, binary or non-regular entries were searched. Exclusions are counted. IO failures, overlong lines and ignore-rule budget failures make the result incomplete; cancellation and output/result limits have explicit flags. A long line stops that file's scan and reports its path and the limit rather than pretending absence. Default output is 64 KiB with a hard 256 KiB cap; line buffers are bounded at 1 MiB, ignore input at 256 KiB/10,000 patterns, and stored issue details at 16 records.

Rooted opens prevent symlink traversal outside the workspace. Directory traversal does not follow symlink entries. A search result is not a filesystem snapshot: concurrent edits can still change a file between discovery and reading. M5 execution isolation remains a separate boundary.

Evidence scenarios: large-source, invalid-glob and cancellation regressions demonstrated failure before this implementation; tests also cover nested negation/overrides, unreadable files, symlink exclusions, long-line/output limits and execution with an empty PATH. The benchmark fixture scans 20 files of 200,000 bytes each and checks complete no-match results.

Reference: https://pkg.go.dev/github.com/go-git/go-git/v5/plumbing/format/gitignore
