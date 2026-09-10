# Explicit file references in staged prompts

Use `@path` to attach a workspace text file to a normal terminal or one-shot
prompt. Quote paths containing spaces, or escape their spaces:

```text
Review @internal/app/service.go
Summarise @"notes/design draft.md"
Compare @notes/design\ draft.md with @notes/revision.md
```

The displayed input stays as typed. The model receives the original prompt
followed by JSON-encoded file snapshots labelled as reference data. Quotes,
newlines and delimiter-like content remain data in that encoding. File contents
are not promoted to a system message. Repeated normalised paths attach once.
Snapshots are read at submission time; later file edits do not change that run's
input. Image references continue through the image attachment parser.

Limits are 256 KiB per text file, 1 MiB of raw text across the prompt and 16
unique text references. Files must be regular UTF-8 text without NUL bytes.
Missing, binary, oversized and external files reject the whole submission.
Symlinks cannot escape the workspace root. Terminal errors retain the draft.

Explicit external-file permission, queued reference resolution,
standalone binary journeys and full native qualification remain pending.

## Tab completion

Place the cursor in an `@file` token and press Tab. A single match replaces that
token while preserving surrounding text and the cursor position. Multiple
matches are listed; type more of the name and press Tab again. Paths containing
spaces are quoted automatically. Directory completions include a slash; quoted
directory paths remain open for further typing.

Completion scans only the relevant workspace directory, reads no file contents,
and refuses traversal or external symlink directories. Scans are bounded to
4096 directory entries and 64 matches; exceeding a limit reports an error.
Results are discarded if the draft, cursor or workspace changes while the scan
runs. Slash-command completion retains its existing Tab behaviour. Tab does not
answer a pending tool approval.
