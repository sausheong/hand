# agcode Phase 3 Design: Sessions

## Context

Phase 1/2 use `session.NewSession(spec.ID, "main")` — in-memory only. Every
restart of agcode starts from a blank conversation, even in the same
directory. This phase wires harness's `session.Store` (JSONL persistence,
already implemented in harness — see `harness/session/store.go`) so
conversations survive restarts.

Key facts about harness's `session.Store`, confirmed by reading the source
before designing this:

- `Store.Load(agentID, key)` already does the right thing for both cases:
  if the file exists it loads all entries; if it doesn't, it returns a
  fresh empty `*Session` already wired via `SetStore` for auto-persist. No
  separate "create" call is needed for the common path.
- `agentID` and `key` must each be a single path component — no `/` or
  `\`, no `.`/`..` (`validateStoreComponent`). A raw workspace path cannot
  be used directly.
- `Session.History()` returns the full root-to-leaf entry path,
  reconstructible into a transcript.

## Goal

One continuous session per workspace directory, auto-resumed on start; a
`--new-session` flag resets it. On resume, the TUI replays prior history
into the transcript instead of starting blank.

## Non-goals (kept from the original phase sketch, made explicit)

- Multiple named sessions per directory, or any session listing/switching
  UI. One directory = one ongoing session. `--new-session` is the only way
  to start over, and it discards the old history rather than branching.
- Any UI for browsing/searching past sessions.

## Design

### `internal/sessionio` (new package)

```go
func StoreDir() (string, error) // ~/.agcode/sessions
func KeyForWorkspace(workspace string) string
```

`KeyForWorkspace` returns a 16-hex-character SHA-256 prefix of the
absolute workspace path. A hash (not a sanitized slug) sidesteps two real
problems with a slug approach: collisions between different paths that
sanitize to the same string, and filenames blowing past reasonable length
limits for deeply nested projects. It trivially satisfies the store's
single-path-component requirement.

### `cmd/agcode/main.go` changes

```go
storeDir, err := sessionio.StoreDir()
store := session.NewStore(storeDir)
key := sessionio.KeyForWorkspace(workspace)
if *newSessionFlag {
    _ = store.Delete(spec.ID, key) // ignore "does not exist"
}
sess, err := store.Load(spec.ID, key)
```

replacing the current `session.NewSession(spec.ID, "main")`. A new
`--new-session` bool flag controls the reset. `spec.ID` (`"agcode"`,
fixed) is reused as the store's `agentID`, so `~/.agcode/sessions/agcode/`
holds one `<hash>.jsonl` per workspace directory ever used.

### `internal/tui` changes

A new pure function, easy to unit-test without a running `Model`:

```go
func ReplayHistory(entries []session.SessionEntry) []string
```

turning stored entries back into the same kind of styled lines the live
path produces — `userLineStyle` for user messages, plain text for
assistant messages (matching the live path's unstyled streamed text),
`toolCallStyle` for tool calls, `toolOKStyle`/`toolErrStyle` for tool
results, and a dim informational line for compaction/meta entries. A new
`(*Model) LoadHistory(entries []session.SessionEntry)` method appends the
replayed lines to `m.transcript` and refreshes the viewport; `main.go`
calls it once, right after `tui.NewModel(rt)`, when `sess.History()` is
non-empty.

## Error handling

A `Store`/`Load` failure (e.g. a corrupted JSONL line) degrades the same
way harness's own `Store` does internally — `Store.Load` already skips
malformed lines with a warning rather than failing outright, so agcode
doesn't need extra handling there. A failure to resolve `StoreDir` (e.g.
`os.UserHomeDir` failing) is a startup error, same treatment as a bad
config file.

## Testing

- `internal/sessionio`: unit tests for `KeyForWorkspace` being stable for
  the same path, different for different paths, and always a valid
  single path component (no separators).
- `internal/tui`: unit tests for `ReplayHistory` covering each
  `EntryType`/role combination, without needing a `Model` or teatest at
  all (it's a pure function).
- An integration-style test wiring a real `session.Store` against a temp
  dir: append a few entries, `Store.Load` a second time, confirm
  `History()` round-trips and `ReplayHistory` produces the same lines the
  live path would have produced for those events.
