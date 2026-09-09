# External-editor workflow

Ctrl+G now opens current input using VISUAL, EDITOR or vi via Bubble Tea ExecProcess. Quoted executables/arguments are parsed without shell evaluation. Input uses a private temporary directory and mode-0600 file; normal completion and failure remove the directory. Valid edited text returns to the textarea without submission. Failed, oversized, invalid UTF-8, non-regular or textarea-incompatible output preserves the original draft.

Tests cover a real executable with spaces in its path, multiline round-trip, permissions and cleanup, shell-literal argument parsing, editor failure, oversized/invalid/symlink output, busy admission and detection of silent textarea line truncation. Final focused tests passed 20 race-enabled repetitions. Full Hand validation passed 619 tests/subtests with zero failures/skips; vet passed.

See [usage](external-editor-usage.md), [aggregate patch](external-editor-integration.patch), and [hashed evidence](external-editor-integration.json).

Native interactive terminal/editor qualification and process-kill cleanup remain pending, as do remaining M3.2 attachment/file-reference work and full acceptance. This is staged integration against unpublished Harness; primary Hand uses v0.3.9.
