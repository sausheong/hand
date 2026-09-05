# hand Phase 6 (Core Capabilities) Implementation Plan

**Goal:** Project-level instructions (`HAND.md`/`AGENTS.md`), an ungated
`search` tool, and local-image vision input from the TUI.

**Spec:** [docs/superpowers/specs/2026-09-05-hand-phase6-design.md](../specs/2026-09-05-hand-phase6-design.md)

## Tasks

1. **`internal/agentio/spec.go`: project instructions** — rename the
   exported `SystemPrompt` constant's text to unexported `baseSystemPrompt`
   (keep `SystemPrompt` exported and pointing at the same text, documented
   as the fallback, since existing callers/tests may reference it
   directly); add `projectInstructionFiles = []string{"HAND.md",
   "AGENTS.md"}` and `BuildSystemPrompt(workspace string) string`, which
   returns `baseSystemPrompt` plus the first found project instruction
   file's content appended underneath (`HAND.md` checked before
   `AGENTS.md`, first hit wins — no merging of both). `BuildAgentSpec`
   calls `BuildSystemPrompt(workspace)` instead of the bare constant.
   **Non-goal, keep out of scope:** no merging of instructions across
   multiple directories (parent dirs, `$HOME`) — one workspace-root file
   only. Unit tests per the spec's Testing section (no file → base prompt
   only; `HAND.md` present → appended; `AGENTS.md` present with no
   `HAND.md` → appended; both present → `HAND.md` wins).
2. **`internal/agentio/search.go` (new file): search tool** — `SearchTool`
   struct (`WorkDir string`) implementing `Name() string` returning
   `"search"`, with JSON-schema parameters `path` (optional, default
   `"."`), `name_glob` (optional), `content` (optional regex), and
   `max_results` (optional, default 100); errors if both `name_glob` and
   `content` are empty. Resolve the starting directory with
   `tool.ExpandHome` + `filepath.Join` against `WorkDir`, then validate it
   with **`tool.ValidatePathInWorkDir(path, WorkDir)`** — **note the
   spec's explicit correction here:** an earlier draft called for reusing
   `agentio.resolvePath` (from `diff.go`) as the boundary check, but that
   function only expands `~`/joins a relative path and is documented as
   "a best-effort preview, not a validation step" — it does not reject an
   out-of-workspace path. `ValidatePathInWorkDir` is the real boundary
   check (the same one `ReadFileTool`/`WriteFileTool`/`EditFileTool`
   already enforce) and must be what actually rejects a `path` like
   `"../../etc"`. Then `filepath.WalkDir` the validated directory,
   skipping `.git`, `node_modules`, `.hand`; for `name_glob` match
   `filepath.Match` against the base name (single-segment only, no `**` —
   documented limitation, not a bug to fix); for `content`, skip files
   over 64 KiB and files that look binary (`\x00` in the first 512
   bytes), compile the regex once, scan line by line, report
   `path:lineNumber:line`. Stop the walk early once `max_results` (across
   all files combined) is reached. Return
   `tool.ToolResult{Output: <newline-joined results, or "no matches">}`.
   **Non-goal, keep out of scope:** no fuzzy/AST-aware search, embeddings
   index, or symbol resolution — plain glob/regex only, same capability
   tier as `grep`/`rg` via `bash` minus the approval prompt. Unit tests
   per the spec's Testing section, in particular the `path: "../../etc"`
   rejection case asserted directly against `tool.ValidatePathInWorkDir`
   (not against `resolvePath`) — this is the regression test for the
   boundary-check mistake in the earlier draft.
3. **`internal/agentio/registry.go`: wire up `SearchTool`** — register
   `&SearchTool{WorkDir: workDir}` in `BuildRegistry` alongside the
   existing tools. Update `internal/agentio/approval.go`'s `gatedTools`
   map by *not* adding `"search"` to it — same trust tier as `read_file`.
   Update `registry_test.go`'s expected-tools list for the new tool.
4. **`internal/agentio/images.go` (new file): image extraction** —
   `imageExts` map (`.png`, `.jpg`, `.jpeg`, `.gif`, `.webp` → MIME type)
   and `ExtractImagePaths(workspace, text string) (string,
   []llm.ImageContent)`, which scans whitespace-delimited tokens in
   `text`, resolves each via `tool.ExpandHome` + join against `workspace`
   (same handling as every other tool's path logic), and for each token
   that resolves to an existing file with a recognized image extension
   **and** passes `tool.ValidatePathInWorkDir(resolved, workspace)`,
   reads it into an `llm.ImageContent` and replaces the token in the
   returned text with `"[image: name.ext]"`. Tokens that don't resolve to
   an in-workspace existing image file (including a read failure such as
   a permission error or race, which is treated the same as "not an
   image") pass through untouched; a message with no image references
   returns `(text, nil)` unchanged. **Correction from an earlier draft of
   this spec, called out explicitly because it's a real privacy issue and
   not cosmetic:** the first version of this function took no `workspace`
   parameter and had no boundary check at all, meaning it would read *any*
   absolute path on the filesystem ending in an image extension (e.g.
   `/etc/foo.png`, a file under `~/.ssh/`) and upload its bytes to a
   third-party LLM API. The fixed design requires `workspace` and calls
   `tool.ValidatePathInWorkDir` before ever treating a token as an image —
   a path outside the workspace is never attached, full stop, regardless
   of extension. **Non-goal, keep out of scope:** no clipboard/terminal
   image paste (`Ctrl+V`); no content-sniffed MIME detection — extension
   only. Unit tests per the spec's Testing section, in particular the
   regression test that an existing image file *outside* the given
   workspace (e.g. `/etc/hostname.png` in a test, or a sibling temp dir)
   is left untouched, not attached.
5. **`internal/tui/model.go`: wire image input into `startRun`** — add a
   `workspace string` field to `Model`, set once from a new second
   argument to `NewModel(rt Runner, workspace string)`. In `startRun`,
   call `cleanText, images := agentio.ExtractImagePaths(m.workspace,
   text)` first; append `cleanText` (not the raw text) to the transcript;
   call `m.rt.Run(ctx, text, images)` — passing the **original** `text`
   (with the real path, not the placeholder) alongside the extracted
   `images`, since the model may benefit from the literal path/filename
   context next to the image bytes. This requires no `Runner` interface
   change since `Run` already takes `images []llm.ImageContent`; only the
   hardcoded `nil` at the call site changes. Update `model_test.go`'s
   `fakeRunner` to record the `images` argument it was last called with,
   and extend/add tests per the spec's Testing section: a temp `.png`
   inside a temp workspace directory referenced by path gets extracted
   (`Run` called with one `ImageContent` whose `MimeType` is
   `"image/png"`, transcript shows the `[image: ...]` placeholder), and
   the same image file located *outside* the temp workspace directory is
   not attached (`Run` called with `images == nil`, transcript shows the
   raw path unchanged).
6. **`cmd/hand/main.go`: pass `workspace` into `NewModel`** — update the
   `tui.NewModel(rt)` call site to `tui.NewModel(rt, workspace)`
   (`workspace` is already in scope there from `os.Getwd()`).
7. **Verify** — go build ./..., go vet ./..., go test ./... across the
   whole module.
