# hand Phase 2 Design: Permissions

## Context

Phase 1 shipped a bare-minimum approval gate: every `write_file`/`edit_file`/
`bash` call prompts yes/no, and nothing is ever remembered — the identical
command prompts again immediately after being approved. This phase adds the
persisted "always allow" mechanism Phase 1 explicitly deferred.

## Goal

Turn the yes/no prompt into a three-way choice — **[y]es once / [a]lways /
[n]o** — where "always" persists per *tool name* (not per exact command) to
a project-local `.hand/settings.json`, so a future run of hand in that
same directory never prompts for that tool again.

Granularity is per-tool-name, not per-command: "always allow bash" means
every future bash call is allowed, matching what a user actually wants when
they get tired of confirming shell commands one at a time. Finer-grained
command-pattern allowlisting (e.g. "allow `git status` but not `rm`") is out
of scope — it would need real command-pattern matching, a meaningfully
bigger feature than this phase.

## Non-goals

- Per-exact-command or command-prefix allowlisting.
- A user-level (as opposed to project-level) allowlist.
- Any UI for reviewing/editing the allowlist other than hand-editing the
  JSON file.
- Revoking an "always allow" from within the running TUI (edit the file or
  delete it to reset).

## Design

### `internal/permissions` (new package)

```go
type Settings struct {
    AlwaysAllow []string `json:"always_allow"`
}

func DefaultPath(workspace string) string // workspace/.hand/settings.json
func Load(path string) (Settings, error)  // missing file -> Settings{}, nil (no auto-create)
func Save(path string, s Settings) error  // creates .hand/ dir as needed

func (s Settings) IsAlwaysAllowed(tool string) bool
```

`Load` deliberately does **not** create the file when missing (unlike
`internal/config.Load`'s auto-create-on-first-run) — a project shouldn't
get a `.hand/settings.json` littered into it just for running hand
once; the file appears only the first time the user actually presses `a`.

A `Store` wraps `Settings` with a mutex and the load path, so the approval
hook has one thing to hold onto across calls within a run:

```go
type Store struct { /* path, mu, cached Settings */ }

func NewStore(path string) (*Store, error) // Load()s once at construction
func (st *Store) IsAlwaysAllowed(tool string) bool
func (st *Store) SetAlwaysAllow(tool string) error // no-op+nil if already allowed; else appends + Save()s
```

### `internal/agentio` changes

`ApprovalRequest.Respond` changes from `chan bool` to `chan Decision`:

```go
type Decision int

const (
    DecisionDeny Decision = iota
    DecisionOnce
    DecisionAlways
)
```

`NewApprovalHook` gains a second parameter, `perms *permissions.Store` (nil
means "no persistence, behave like Phase 1" — kept nil-safe the way
harness's own optional plug-points are, so tests that don't care about
permissions don't need to construct a Store). Before sending a prompt, it
checks `perms.IsAlwaysAllowed(name)`; if true, allows immediately with no
prompt, matching how `gatedTools` itself is checked. On `DecisionAlways`,
it calls `perms.SetAlwaysAllow(name)` before allowing — a persistence
failure there is logged but does not deny the call the user already
approved.

### `internal/tui` changes

The pending-approval branch in `handleKey` grows a third case (`a`/`A` →
`DecisionAlways`), and the transcript line on that path reads
`"always allowed: <tool>"` (styled like `approvedStyle`, since it's a
strictly stronger approval than a one-time yes). The status line's prompt
text becomes `"Allow <tool>? [y]es / [a]lways / [n]o"`.

### `cmd/hand/main.go` changes

Build a `permissions.Store` at `permissions.DefaultPath(workspace)` right
alongside the registry/spec construction, and pass it into
`agentio.NewApprovalHook`. A `Store` construction failure (e.g. malformed
existing `settings.json`) is a startup error, same treatment as a bad
config file.

## Testing

- `internal/permissions`: unit tests for `Load` returning empty on a
  missing file (no file created), `Save`/`Load` round-trip,
  `IsAlwaysAllowed`, and `Store.SetAlwaysAllow`'s persist-then-reflect
  behavior (a second `Store` opened against the same path sees the
  earlier `Store`'s addition).
- `internal/agentio`: extend the existing approval-hook tests for the
  `Decision` enum instead of `bool`; add cases for
  `perms.IsAlwaysAllowed` short-circuiting the prompt, and for
  `DecisionAlways` persisting before returning Allow.
- `internal/tui`: extend the existing approval-prompt teatest cases for
  the three-way choice; add a case pressing `a` and asserting the
  transcript shows "always allowed" and the prompt text shows the three
  options.
