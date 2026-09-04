# agcode Phase 4 Design: One-shot mode + polish

## Context

Phases 1-3 cover the interactive TUI, persisted permissions, and session
resume. This phase adds a non-interactive `-p` mode for scripting, and
closes a gap discovered while designing it: **the approval prompt
currently shows only the tool name, never what it's about to do** —
`Allow bash? [y]es / [a]lways / [n]o` with no visible command, and no
preview at all for file writes/edits. Diff rendering was already in
scope; showing the raw command for `bash` is the same gap for a
different tool and is fixed alongside it.

## Goals

1. `-p "prompt"` runs one turn non-interactively, prints assistant text to
   stdout, and exits — no TUI.
2. One-shot mode has no one to prompt, so it consults Phase 2's
   persisted allowlist: an already always-allowed tool proceeds, anything
   else is denied with a message pointing at how to approve it. `--yes`
   (valid only with `-p`) bypasses this for scripting.
3. The approval prompt shows a preview of what it's approving: the literal
   command for `bash`, a diff for `write_file`/`edit_file` computed from
   the tool's *input* against the file currently on disk (not from
   executing anything).

## Design

### `internal/agentio`

`ApprovalRequest` gains a `Preview string` field — plain text, no
styling (styling is the TUI's job, matching how `agentio` already stays
framework-agnostic elsewhere). Empty means "nothing to preview" (never
actually empty in practice now that `bash` shows its command too).

A new `diff.go` builds it:

```go
func buildPreview(workspace, tool string, input json.RawMessage) string
```

- `bash`: unmarshals `{command}`, returns `"$ " + command`.
- `write_file`: unmarshals `{path, content}`, resolves `path` against
  `workspace` the same way `tools/file` does (reusing
  `harness/tool.ExpandHome`), reads the file's current content (empty if
  it doesn't exist yet — a new file), and diffs old vs. `content`.
- `edit_file`: unmarshals `{path, old_string, new_string}`, reads the
  current file, and diffs old content against old content with
  `old_string` replaced once by `new_string` — mirroring what the tool
  itself will do, without executing it. If the file can't be read, the
  preview is silently omitted (empty string) rather than blocking the
  prompt — a best-effort preview, not a validation step; the tool's own
  execution still enforces the real invariants (`old_string` must match
  exactly once).

The diff itself (`lineDiff(oldText, newText string) string`) is a
common-prefix/common-suffix line diff with 2 lines of context — not a
minimal LCS diff, deliberately: LLM-driven edits are overwhelmingly
"change one contiguous region," which this handles well, and it's O(n)
with no pathological blowup risk, unlike a naive LCS table. Output lines
are prefixed `"+ "`/`"- "`/`"  "` (added/removed/context), which the TUI
colors by prefix. Large changes (combined line count over a cap) skip the
line diff and render a one-line size summary instead, rather than
dumping thousands of lines into a terminal prompt.

`NewApprovalHook` gains a `workspace string` parameter to resolve
relative paths for the preview. `NewOneShotApprovalHook(perms
*permissions.Store, autoApprove bool)` is the one-shot equivalent: no
`Sender`, no prompt, no preview — either `autoApprove` or
`perms.IsAlwaysAllowed` decides, everything else is denied with a
`Reason` explaining how to approve it interactively.

### `internal/tui`

The approval panel becomes a small block instead of one status line: the
(colored, by line prefix) preview, then the `[y]es / [a]lways / [n]o`
question. Because the preview's height varies (a diff can be several
lines, a bash command is one line), the viewport's height can no longer
be a fixed constant computed only on `WindowSizeMsg` — `resize()`'s
layout math moves into `refreshViewport()` itself, computed fresh every
time the transcript or approval state changes, using terminal
dimensions cached in the `Model` from the last `WindowSizeMsg`. This is
the one non-obvious mechanical consequence of adding a variable-height
preview.

### `cmd/agcode/main.go`

Two new flags: `-p` (string, the one-shot prompt) and `--yes` (bool,
error at startup if set without `-p`). When `-p` is set, `main.go` skips
building the TUI (`tea.Model`, `*tea.Program`, `programSender`) entirely
— the one-shot path only needs the `Runtime`, mirroring harness's own
`examples/minimal`'s non-streaming print loop: drain `EventTextDelta` to
stdout, return the first `EventError.Error` (if any) as the process's
exit error. Session persistence and `--new-session` behave identically
in both modes — a one-shot run's turn is still saved.

## Testing

- `internal/agentio`: `lineDiff` unit tests (added lines, removed lines,
  changed region, new file from empty, identical content, the size-cap
  fallback). `buildPreview` tests per tool using a temp workspace
  directory. `NewOneShotApprovalHook` tests mirroring `NewApprovalHook`'s
  existing coverage (ungated always allowed, gated denied by default,
  gated allowed when already always-allowed, gated allowed with
  `autoApprove`).
- `internal/tui`: extend the approval-prompt teatest cases to assert the
  preview text renders (a bash command, a diff's `+`/`-` lines).
