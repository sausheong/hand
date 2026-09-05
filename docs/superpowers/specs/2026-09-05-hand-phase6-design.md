# hand Phase 6 Design: Core Capabilities — Project Instructions, Search Tool, Image Input

## Context

Three gaps keep hand from feeling like a complete coding agent, each
found by reading what's actually wired today:

1. `agentio.SystemPrompt` is a fixed constant, always non-empty, always
   passed as `AgentSpec.SystemPrompt`. Harness's own
   `BuildStaticSystemPrompt` only reads a project file (`IDENTITY.md`)
   `if systemPrompt == ""` — an XOR, not additive — so that path never
   fires for hand. There is no way for a project to tell hand about its
   own conventions.
2. The only tools are `read_file`, `write_file`, `edit_file`, `bash`,
   `web_fetch`, `web_search`, `todo_write`. There is no read-only
   search/list tool, so routine exploration (`ls`, `grep`) goes through
   the fully-gated `bash` tool and prompts for approval every time.
3. `tui.Model.startRun` always calls `m.rt.Run(ctx, text, nil)` — the
   `images` parameter is hardcoded nil even though `Runner.Run` and
   harness both support `[]llm.ImageContent` vision input. There is no
   way to show hand a screenshot or image from the TUI.

## Goals

1. hand reads project-level instructions from `HAND.md` or `AGENTS.md` at
   the workspace root and appends them to its own fixed system prompt.
2. Add an ungated `search` tool (filename glob + content grep) so
   read-only exploration doesn't trigger approval prompts.
3. Let a user attach a local image by referencing its file path in a chat
   message; hand detects it, reads the file, and sends it as vision
   input alongside the remaining text.

## Non-goals

- Merging instructions from multiple directories (parent dirs, `$HOME`).
  One workspace-root file only, matching where `.hand/settings.json`
  already lives.
- Clipboard/terminal image paste (`Ctrl+V`). Terminal image-paste is
  emulator-specific (Kitty graphics protocol, iTerm2 proprietary
  escapes, OSC 52) — this phase deliberately picks the portable path
  that works in every terminal: a file already on disk, referenced by
  path (which is what dragging a file into many terminal emulators
  already inserts as text).
- A fuzzy or AST-aware code search (an embeddings index, symbol
  resolution). `search` is a plain glob/regex-over-files tool — the same
  capability tier as running `grep`/`rg` via `bash`, minus the approval
  prompt.
- Content-sniffed MIME detection for images — extension-based only
  (`.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`).

## Design

### `internal/agentio`: project instructions

```go
// baseSystemPrompt is hand's fixed identity text (previously the
// exported SystemPrompt constant).
const baseSystemPrompt = `You are Hand, ...`

// projectInstructionFiles are checked in order; the first one found
// wins. HAND.md is hand-specific and takes precedence over the more
// widely-adopted AGENTS.md convention, so a repo that already has an
// AGENTS.md for other tools works with hand too without duplication.
var projectInstructionFiles = []string{"HAND.md", "AGENTS.md"}

// BuildSystemPrompt returns hand's fixed identity plus, if present, one
// project instruction file's content appended underneath.
func BuildSystemPrompt(workspace string) string {
    for _, name := range projectInstructionFiles {
        data, err := os.ReadFile(filepath.Join(workspace, name))
        if err != nil {
            continue
        }
        return baseSystemPrompt + "\n\n---\n\nProject instructions (" + name + "):\n\n" + string(data)
    }
    return baseSystemPrompt
}
```

`BuildAgentSpec` calls `BuildSystemPrompt(workspace)` instead of the
former bare constant. `SystemPrompt` stays exported (some callers/tests
may reference the base text directly) but is documented as the
*fallback*, not the whole story.

### `internal/agentio`: search tool (new file `search.go`)

```go
// SearchTool finds files by name glob and/or content regex under
// WorkDir. Read-only by construction — never registered in gatedTools.
type SearchTool struct { WorkDir string }

func (t *SearchTool) Name() string { return "search" }
```

Parameters (JSON schema, matching the existing tools' style):

```json
{
  "path": "string, optional, directory to search under (default \".\")",
  "name_glob": "string, optional, filename glob e.g. \"*.go\"",
  "content": "string, optional, regex to grep file contents",
  "max_results": "integer, optional, default 100"
}
```

At least one of `name_glob`/`content` is required (error otherwise). The
starting directory is resolved with `tool.ExpandHome` + `filepath.Join`
against `WorkDir` (mirroring how `tools/file`'s own tools build the
path), then validated with **`tool.ValidatePathInWorkDir(path, WorkDir)`**
— the actual security boundary `ReadFileTool`/`WriteFileTool`/`EditFileTool`
enforce at execute time. This is a correction from an earlier draft of
this spec, which called for reusing `agentio.resolvePath` (from
`diff.go`) as the boundary check — that function only expands `~` and
joins a relative path; it does **not** validate the result stays inside
`WorkDir` (its own doc comment says it's "a best-effort preview, not a
validation step," built only for cosmetic approval-prompt previews). A
`path` of `"../../etc"` must be rejected by `SearchTool`, and only
`ValidatePathInWorkDir` actually does that.

`filepath.WalkDir` then runs under the validated directory, skipping
`.git` and other common VCS/build dirs (`.git`, `node_modules`, `.hand`).
For each regular file:

- Skip files over a size cap (64 KiB, matching the rough order of
  magnitude of typical source files) when `content` is set — an
  unbounded read-and-scan would happily try to load a multi-GB log file
  sitting in the workspace into memory. `name_glob`-only matching never
  reads file contents, so the cap doesn't apply to it.
- If `name_glob` is set, match it against the file's base name via
  `filepath.Match` (no `**`/multi-segment glob support — matches a
  single path segment, same as the stdlib function; documented as a
  known limitation rather than reimplementing a glob engine).
- If `content` is set, skip files that look binary (a `\x00` byte in the
  first 512 bytes — the same heuristic `file`/`grep` use), compile the
  regex once per call, and scan line by line, reporting
  `path:lineNumber:line` for each match.

Results are capped at `max_results` total (across all files combined,
not per file) — once reached, the walk stops early rather than
collecting everything and truncating at the end, so a huge repo doesn't
pay to enumerate matches it will discard.

Returns `tool.ToolResult{Output: <newline-joined results, or "no matches">}`.

Registered in `BuildRegistry` alongside the others. **Not** added to
`gatedTools` in `approval.go` — same trust tier as `read_file`.

### `internal/agentio`: image input (new file `images.go`)

```go
// imageExts maps a recognized file extension to its MIME type.
var imageExts = map[string]string{
    ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
    ".gif": "image/gif", ".webp": "image/webp",
}

// ExtractImagePaths scans text for whitespace-delimited tokens that
// resolve (via tool.ExpandHome + join against workspace, same as every
// other tool's path handling) to an existing file, inside workspace,
// with a recognized image extension. Each match is read into an
// llm.ImageContent and the token is replaced in the returned text with a
// short "[image: name.ext]" placeholder — so the remaining text still
// reads naturally and the transcript shows what was attached. Tokens
// that aren't in-workspace existing image files pass through untouched;
// a message with no image references is returned unchanged with a nil
// slice.
func ExtractImagePaths(workspace, text string) (string, []llm.ImageContent)
```

**Correction from an earlier draft of this spec:** the first version of
this function took no `workspace` parameter and had no boundary check at
all — meaning it would have read *any* absolute path on the filesystem
that happened to end in an image extension (`/etc/foo.png`, a file under
`~/.ssh/`, anything readable by the process) and silently uploaded its
bytes to a third-party LLM API. That's a real privacy leak, not a
cosmetic gap. The fixed design requires `workspace` and calls
`tool.ValidatePathInWorkDir(resolved, workspace)` (the same real boundary
check `SearchTool` uses above) before treating a token as an image —
**a path outside the workspace is never attached, full stop**, regardless
of whether it looks like a valid image. This does mean a user can't
attach, say, a screenshot saved to `~/Desktop` unless it's inside the
current workspace — an acceptable, safety-first restriction for this
phase; loosening it later (e.g. an explicit `--allow-path` escape hatch)
is a separate decision, not a default.

A read failure on an in-workspace path (permission, race) is treated the
same as "not an image" — leaves the token untouched rather than erroring
the whole message.

### `internal/tui` changes

`startRun` calls `agentio.ExtractImagePaths(m.workspace, text)` first —
`Model` gains a `workspace string` field, set once from `NewModel`'s
caller (`main.go` already has `workspace` in scope where it constructs
`tui.NewModel(rt)`; that call gains a second argument):

```go
func (m *Model) startRun(text string) tea.Cmd {
    cleanText, images := agentio.ExtractImagePaths(m.workspace, text)
    m.transcript = append(m.transcript, userLineStyle.Render("> "+cleanText))
    m.textarea.Reset()
    m.running = true
    ...
    events, err := m.rt.Run(ctx, text, images) // note: original text, not cleanText, goes to the model
    ...
}
```

The **placeholder-substituted** text (`cleanText`) is what the user sees
in the transcript; the **original** text (with the real path) is what's
sent to the model as the accompanying text, since the model may benefit
from the literal filename/path context alongside the image bytes. This
requires no `Runner` interface change — `Run`'s signature already takes
`images []llm.ImageContent`; only the hardcoded `nil` at the call site
changes.

## Testing

- `internal/agentio`:
  - `BuildSystemPrompt`: no file → base prompt only; `HAND.md` present →
    appended; `AGENTS.md` present (no `HAND.md`) → appended; both
    present → `HAND.md` wins.
  - `SearchTool`: `name_glob` match, `content` regex match with correct
    `path:line:text` format, `max_results` cap stops the walk early,
    binary file skipped for `content` search, an oversized file skipped
    for `content` search (not for `name_glob`), missing both parameters
    errors, and — the case that matters most given the earlier boundary-
    check mistake in this spec — `path: "../../etc"` (or any path
    resolving outside `WorkDir`) is rejected by
    `tool.ValidatePathInWorkDir`, asserted directly against that real
    check rather than against `resolvePath`.
  - `ExtractImagePaths`: an existing in-workspace `.png` path is extracted
    and replaced with a placeholder; a non-existent path is left alone; a
    non-image extension is left alone; multiple images in one message all
    get extracted; a message with no paths returns `(text, nil)`
    unchanged; **an existing image file *outside* the given workspace
    (e.g. `/etc/hostname.png` in a test, or a sibling temp dir) is left
    untouched, not attached** — this is the regression test for the
    privacy leak the first draft of this function had.
- `internal/tui`: extend `fakeRunner` to record the `images` argument
  it was last called with. A test creates a temp `.png` file inside a
  temp workspace dir, types a message containing its path, presses
  enter, and asserts (a) `Run` was called with one `ImageContent` whose
  `MimeType` is `"image/png"`, and (b) the transcript shows the
  `[image: ...]` placeholder instead of the raw path. A second test does
  the same with the image file *outside* the temp workspace dir and
  asserts `Run` was called with `images == nil` and the transcript shows
  the raw path unchanged (not attached, not stripped).
