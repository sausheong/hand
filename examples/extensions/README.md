# Task-note extensions

The Go and Python implementations demonstrate the same version-1 JSONL peer:
`note` asks a structured free-text question, saves the answer through the host's
revision-checked session state, and returns a declarative text block. A context
transform trims surrounding whitespace from editable contributions. User,
system and policy contributions remain protected by the host.

Build Go from the repository root:

```sh
go build -o /tmp/hand-task-note ./examples/extensions/go-task-note
```

Python requires Python 3 and uses only its standard library. It is launched with
`-I -B ${package}/main.py`: isolated interpreter configuration, no bytecode writes,
and a reviewed snapshot of `python-task-note/main.py`. Both peers reserve stdout
for protocol frames; diagnostics go to stderr. Running either directly waits for
host protocol input. They are not standalone terminal commands.

The host must explicitly review and admit the name `task-note` with exactly these
capabilities: `commands`, `questions`, `state`, and `context.transform`. Bind state
to the installed package identity and selected session, and route `user.question`
to the host question broker. Capability declarations do not grant filesystem,
network or process authority. These examples request none of those host APIs.
Unrestricted host execution can nevertheless access the host OS; choose a
supported container factory when isolation is required. The host factory does
not snapshot interpreter runtime libraries. Missing or incompatible runtimes
must fail activation rather than fall back to another execution boundary.

Run the integration journey:

```sh
go test -race -count=1 ./internal/extensions -run '^TestGoAndPythonTaskNoteExamples$'
```

This test requires Go and Python 3 on PATH and fails if either is unavailable.
It compiles the Go example, reviews each executable/source configuration,
launches both through the host factory, supplies a validated structured answer,
checks the presentation and protected context, then restarts each peer and disk
session to verify persisted state. Its admission callback and answer responder
are test-owned: it does not prove application trust prompts or terminal question
interaction. Product CLI/TUI activation wiring remains in development.

Use MCP when you only need to expose tools. This extension protocol is intended
for commands, host-managed interaction, lifecycle events and context transforms.
The Python example is a small peer implementation, not a general protocol SDK;
the host remains responsible for enforcing protocol bounds and permissions.

## Additional tool-policy veto

Build `go build -o /tmp/hand-tool-policy ./examples/extensions/go-tool-policy`.
Review it as `tool-policy` with capability `policy.check` and `mandatory: true`.
Its optional `--deny-path PATH` argument defaults to `protected.txt`. It denies a
`tool.execute` request when its input object's `path` exactly matches that string.
This is a small protocol example of an additional veto, not canonical filesystem
path enforcement; Hand's existing path validation and permission policy remain
responsible for filesystem boundaries. The callback receives the tool name as
`resource` and the bounded original JSON arguments as `input`.

Host denials are final and do not dispatch the extension check. An explicit peer
denial always denies; a mandatory peer failure also denies. Existing tool-list
filtering is preserved. Configure the policy before a turn starts; reload occurs
only at an idle application boundary.

### Verification hook (tracked Git whitespace)

Build `go build -o /absolute/verification-hook ./examples/extensions/go-verification-hook` and review it through the normal explicit extension startup path with name `verification-hook` and capabilities `commands` and `lifecycle`. It requires a host Git executable on PATH and a Git workspace. Its `verify-whitespace` command accepts no arguments. The same check runs as a `run.finish` observer.

The example executes fixed `git diff --no-ext-diff --no-textconv --check` commands for unstaged and staged changes, sharing a one-second deadline and bounded output capture. It invokes no shell, external diff helper, text converter or repository hook. Host execution approval is still required; protocol capability declarations are not an OS sandbox.

A pass means only that these tracked whitespace checks passed. It is not evidence that tests passed, untracked files are valid, or the implementation is complete. Failed finish observation produces a diagnostic; it does not rewrite Hand's terminal outcome. Use Hand's reviewed verification profiles for project test commands and durable verification records.

### Tool result viewer

Build `go build -o /absolute/tool-viewer ./examples/extensions/go-tool-viewer`. Review it under the name `tool-viewer` with capabilities `commands` and `presentation`. Run `/extension tool-viewer view-tool {"tool":"read_file","output":"Example result"}` or send the equivalent extension command through RPC. It accepts an explicit JSON record with `tool`, `output` and optional `error`, capped at 8 KiB. The viewer preserves that record in a JSON code block, escaping terminal control characters, and labels it as supplied data. It does not automatically intercept tool events, execute tools, verify a claimed result or change the run outcome.

### When to use MCP or an extension

Use MCP to expose service tools or data through a standard server protocol. Use a Hand extension when the integration needs Hand-specific commands, structured questions, session state, context transforms, policy vetoes or typed presentation. An extension can complement an MCP tool by displaying an explicitly supplied result; the viewer example does not create an automatic MCP event subscription. Both approaches still operate under the configured permissions and execution boundary.
