# Instructions and skills — development integration

These behaviours are in the staged implementation and integration patches; they
are not yet a released Hand/Harness combination.

## Instructions

At startup, Hand reads personal guidance from `~/.hand/HAND.md` (or
`~/.hand/AGENTS.md`), then filesystem ancestors from broadest to the workspace.
At each directory, an existing `HAND.md` takes precedence over `AGENTS.md`.
A rejected or unreadable preferred file produces a diagnostic; Hand does not
silently substitute the other file. Collisions report both source paths.

User requests take precedence over project guidance. Nearer applicable project
guidance takes precedence over broader guidance and personal defaults. Guidance
does not grant tool permissions.

A successful `read_file` loads applicable guidance below the workspace down to
the file's parent. It returns the guidance before the file contents, with source
metadata. It does not load unrelated sibling guidance or repeat startup guidance.
Later reads discover updated files. Reads through the isolated worker perform
this discovery inside that worker's filesystem boundary.

Each instruction body is limited to 32 KiB; each discovery operation has a
128 KiB body limit and 64-level ancestry limit. Exclusions are visible. Leaf
symlinks and nonregular instruction files are excluded. External directory
replacement during discovery is not yet fully guarded.

Direct write/edit/search guidance, context inspection, and large-result retention
qualification are still pending. `/reload` currently refreshes skills only.

## Skills

Discovery precedence, highest first:

1. Workspace `.hand/skills`
2. Personal `~/.hand/skills`
3. Workspace `.agents/skills`
4. Personal `~/.agents/skills`

Keeping both existing Hand locations ahead of new conventional locations prevents
migration from silently replacing a previously selected skill. Index entries
include source paths; collisions identify the winner. Self-authoring still writes
to workspace `.hand/skills` only.

`load_skill` includes the full body, source path, and resource base directory.
Resolve `references/example.md` against that base, subject to normal file access
permissions. A resource path is guidance, not permission to execute a script.
Isolated workers can discover only stores available inside their boundary.

Discovery excludes a store with more than 256 directory entries and skill bodies
over 32 KiB. Invalid or unreadable selected bodies produce index diagnostics.

The runtime refreshes the index between model requests after prior tool execution
has joined, so self-authored skills appear without a restart. `/reload` refreshes
explicitly while idle. RPC clients negotiate `hello`, then send `skills.reload`
with `{}` parameters and a new request ID. Duplicate IDs replay the original
control response; use a fresh ID for another refresh.

## Reproducible worker check

Build the staged `cmd/hand-tool-worker`, then run:

```sh
python3 scripts/check_guidance_worker.py \
  --binary /absolute/path/to/hand-tool-worker \
  --evidence /absolute/path/to/new-evidence-directory
```

The runner uses four native processes and no provider calls. It preserves request
and response bytes, fixture files, runner/binary hashes, and a result manifest.
This check does not qualify container isolation or the final acceptance candidate.
