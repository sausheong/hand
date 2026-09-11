# Hand

Hand is a minimal, extensible terminal coding agent. Point it at a directory
and it can read, search, edit, and run commands in that workspace on your
behalf — with a lightweight approval gate for anything that changes files or
runs shell commands.

Hand is deliberately small: one binary, one config file, no daemon, no
project scaffolding forced onto your repo. It's built on top of
[`harness`](https://github.com/sausheong/harness), a Go library for building
LLM agents.

> **Current release: v0.3.4.** This release pins Harness v0.4.2 and supports
> macOS and Linux on AMD64 and ARM64. Hand's interfaces remain pre-1.0; pin
> the release you deploy and follow the migration and rollback guide when
> upgrading.

## What's in v0.3.4

- Empty or truncated model responses no longer report successful completion. Generic empty responses get one bounded retry; output-limit failures explain what happened.
- Set the output allowance with `--max-output` or `max_output` in configuration. `/timing` records the allowance and provider stop reason.
- Bash calls show the first five command lines as soon as the complete arguments arrive.
- Raw interactive diagnostics go to private `~/.hand/logs/` files; a divider and spacing separate results, status and input.
- Press **Esc** to cancel the current turn, including while an approval or output viewer is open.
- `/permissions skip` and `/permissions ask` control approvals for the current session. `--dangerously-skip-permissions` enables the same temporary setting at launch.
- Routine skipped context-cleanup messages stay quiet, and truncated summaries preserve the original history.

See [session approvals](docs/session-approval.md), [output limits](docs/empty-response-fix.md), and [transcript navigation](docs/transcript-navigation.md).

## What's in v0.3.3

- Mouse wheel and trackpad scrolling; `/mouse off` restores native text selection.
- Readable skills list with bold blue names, separate descriptions and source paths, and more spacing.
- Larger skill files load on demand, with a 1 MiB limit and a lightweight startup index.
- `/timing` shows where a turn spent its time; detailed reports are saved in the session.
- Simpler completion, error and execution-mode messages. The configured context limit is visible before usage is known.

See [transcript navigation](docs/transcript-navigation.md), [turn timing](docs/turn-timing.md), and [skill loading](docs/package-skills.md).

## What's in v0.3.2

- Direct persistent Bash approval: `/permissions allow bash --project`.
- Inspect saved grants with `/permissions`; revoke with `/permissions revoke project-bash`.
- Clearer approval labels distinguish the exact command from project-wide Bash approval.

## What's in v0.3.1

This patch fixes repeated tool-call finish frames causing cancelled runs,
preserves meaningful protocol errors, and avoids preventive compaction when
conversation history is too short. Default OpenRouter profiles now discover
exact model context limits from the public catalogue and cache them for fifteen
minutes. Explicit limits take precedence; unavailable metadata retains the
labelled conservative fallback, inspectable with `/model`.

## What's in v0.3.0

v0.3.0 turns Hand from a compact interactive agent into a fuller coding-agent
runtime while retaining a single native CLI:

- Explicit host or Docker-container tool execution. Container mode uses an
  immutable image and worker digest, an allowlisted environment, explicit
  mounts and limits, and never falls back to host execution.
- Durable sessions with resume, naming, branching, export, attachments,
  checkpoints, restore previews and interrupted-operation recovery.
- Scoped, revocable permissions; mandatory verification profiles; Stop-hook
  goal loops; token, cost, run and wall-clock budgets.
- Mid-run steering and follow-up queues, background processes, full output
  artifacts, context pins and inspectable agent state.
- Named provider profiles with explicit context, output, reasoning and input
  capabilities, plus verified local-model metadata discovery.
- Versioned JSONL and RPC automation, an embeddable Go SDK, and reviewed,
  digest-pinned packages and extensions.

Container isolation applies to commands run through the tool backend. Local or
remote MCP servers, hooks and explicitly approved host extensions execute
outside that container; Hand requires those boundaries to be acknowledged in
container mode. See [Execution isolation](#execution-isolation).

## Installing

The CLI is a native Go executable. Distribution archives also include a Linux
tool worker for optional container execution. Shell tools and optional
servers/extensions have their own runtime requirements.

### Build from source

You need [Go 1.25.1+](https://go.dev/dl/).

```sh
git clone https://github.com/sausheong/hand.git
cd hand
make build          # writes bin/hand
./bin/hand
```

Or install straight into `$GOPATH/bin` (or `$GOBIN`) without cloning:

```sh
go install github.com/sausheong/hand/cmd/hand@latest
hand
```

Make sure that directory is on your `PATH`.

### Download a prebuilt binary

Choose an archive and `SHA256SUMS` from the same
[release](https://github.com/sausheong/hand/releases). Supported targets are
`darwin-amd64`, `darwin-arm64`, `linux-amd64` and `linux-arm64`.
Follow [installation, upgrades and rollback](docs/installation-and-upgrades.md)
to verify the checksum before extraction, preserve the complete CLI/worker
installation, and back up existing sessions before upgrading.


## Setting up an API key

Hand doesn't store API keys — each provider reads its key from its own
environment variable:

| Provider    | Environment variable  |
|-------------|------------------------|
| Anthropic   | `ANTHROPIC_API_KEY`    |
| OpenAI      | `OPENAI_API_KEY`       |
| Gemini      | `GEMINI_API_KEY`       |
| OpenRouter  | `OPENROUTER_API_KEY`   |
| LiteLLM     | `LITELLM_API_KEY` (optional — many self-hosted proxies don't enforce auth) |
| Local       | none — local servers don't authenticate requests |

Export the one matching your default model before running Hand, e.g.:

```sh
export ANTHROPIC_API_KEY=sk-ant-...
```

**LiteLLM and OpenRouter** are aggregators, not single vendors — each
fronts dozens to hundreds of underlying models behind one OpenAI-compatible
endpoint, so supporting them once gets you that whole catalog instead of a
one-off integration per vendor. OpenRouter needs no other setup
(`--model openrouter/<catalog-id>`); LiteLLM is self-hosted, so you also
need `--base-url` (or `base_url` in config) pointing at your proxy —
there's no public default endpoint to fall back to.

> **Watch the prefix.** OpenRouter's own catalog ids look like
> `anthropic/claude-sonnet-4-5` or `google/gemini-3-pro` — a vendor name
> followed by a model name, the same shape Hand uses for `provider/model`.
> The *first* segment you give Hand always selects Hand's own provider, so
> to pick that catalog entry through OpenRouter you must repeat the
> aggregator prefix: `--model openrouter/anthropic/claude-sonnet-4-5` (or
> `/model openrouter/anthropic/claude-sonnet-4-5` at the prompt). Drop the
> `openrouter/` and Hand switches straight to its own `anthropic` provider
> — a real vendor, so it won't be rejected as "unknown provider" — and
> calls the real Anthropic API directly with whatever model string
> followed, which usually isn't a valid Anthropic model id.

**Local** models run on your own machine — no key, no network round-trip
beyond it. `--model local/<model-name>` talks to
`http://localhost:11434/v1` by default (Ollama's endpoint — e.g.
`local/qwen2.5:3b`, matching what `ollama list` shows); pass `--base-url`
for a different port or a different local server entirely (LM Studio,
llama.cpp's server, vLLM, or anything else exposing an OpenAI-compatible
endpoint).

## Running it

Run `hand` from the directory you want it to work in — that directory is the
workspace, and every file tool is confined to it:

```sh
cd my-project
hand
```

This opens an interactive terminal UI. Type a request and press Enter; Hand
streams its response and shows each tool call it makes along the way.
Assistant responses are rendered as Markdown (headings, bold/italic, lists,
code blocks) — the model's raw formatting shows up styled, not as literal
`**`/`#`/`` ` `` characters. The style defaults to `dark`; see `--markdown-style`
below to change it.

### One-shot mode

For scripting or CI, run a single turn non-interactively with `-p`:

```sh
hand -p "list all TODO comments in this repo"
```

By default, any gated tool call (see below) is denied in one-shot mode
rather than prompting — there's no one to ask. Pass `--yes` to auto-approve
everything for that run instead:

```sh
hand -p "fix the failing test in pkg/foo" --yes
```

One-shot exit codes are stable for scripts:

| Code | Meaning |
|------|---------|
| `0` | Answer completed; all configured mandatory Stop validators passed |
| `2` | Invalid invocation or configuration |
| `3` | Required prompt/Stop validation failed |
| `4` | Tool-turn or goal-iteration limit exhausted |
| `5` | Provider, runtime or infrastructure failure |
| `130` | Interrupted or cancelled |

A normal answer without mandatory validators is **not verified coding success**.
Passing validators establishes only what those checks actually test. Reaching
`--max-turns` or `--max-iterations` never returns success. SIGINT and SIGTERM
cancel an active one-shot run, drain its events and allow runtime cleanup
before exit, including during MCP startup.

### Flags

| Flag               | Description |
|---------------------|-------------|
| `--model`           | `provider/model` to use for this run, e.g. `anthropic/claude-sonnet-5` — overrides `~/.hand/config.json` |
| `--profile`         | Named provider/model profile from config |
| `--base-url`        | Custom API base URL — required for `litellm`, optional for `openai`/`openrouter`, not supported for `gemini` |
| `--context-limit`   | Explicit active context limit for this invocation |
| `--max-output` | Output token allowance per model request for this invocation |
| `--dangerously-skip-permissions` | Automatically approve all tools for this session without saving grants; execution isolation remains active |
| `--reasoning`       | `off`, `low`, `medium` or `high`; the selected profile must declare support |
| `--max-turns`       | Cap the agent's tool-use loop for this run (default: 50, or `max_turns` in config) |
| `--fallback-model`  | `provider/model` to retry against on a transient provider error, same provider as `--model` |
| `--markdown-style`  | Glamour style for rendering assistant Markdown: `dark`, `light`, `ascii`, `notty`, `pink`, `dracula`, `tokyo-night` (default `dark`) — overrides `~/.hand/config.json` |
| `--compaction-threshold` | Fraction (0-1] of the context window that triggers preventive compaction (default `0.4`) — overrides `~/.hand/config.json` |
| `--max-iterations`  | Cap how many turns a [Stop-hook goal loop](#goal-loop) may chain automatically (default `10`, or `max_goal_iterations` in config) |
| `--new-session`     | Create a new session while preserving existing history |
| `--session`         | Resume a saved session by stable ID |
| `--export-session`  | Export a stopped session to a new JSONL path without starting a model |
| `--checkpoint-dir`  | Store bounded run checkpoints in a private directory outside the workspace |
| `--rpc`             | Serve the versioned JSONL RPC protocol on stdin/stdout |
| `--jsonl`           | Emit versioned JSONL events in one-shot mode |
| `-p "<prompt>"`     | Run one turn non-interactively and exit (no TUI) |
| `--yes`             | Auto-approve all gated tool calls for this run (only valid with `-p`) |

## Slash commands

Inside the interactive UI, a message starting with `/` runs a command
instead of being sent to the model. Typing `/` shows an auto-complete
dropdown (arrow keys to move, Tab or Enter to fill it in).

| Area | Commands |
|------|----------|
| Help and display | `/help`, `/clear`, `/output`, `/exit` (`/quit`) |
| Models and context | `/model`, `/profile`, `/context`, `/compact`, `/summarizer`, `/summarizer-confirm`, `/summarizer-follow`, `/pins`, `/pin`, `/unpin` |
| Sessions | `/new`, `/resume`, `/name`, `/tree`, `/fork`, `/export` |
| Work in progress | `/steer`, `/followup`, `/queue`, `/process`, `/state` |
| Safety and evidence | `/boundary`, `/permissions`, `/changes`, `/restore-preview`, `/restore-confirm`, `/restore-cancel`, `/recoveries`, `/recovery-resolve`, `/verify`, `/verify-confirm`, `/verify-check`, `/verify-list`, `/verify-delete` |
| Usage and budgets | `/usage`, `/budget`, `/budget-tokens`, `/cost`, `/prices`, `/prices-review`, `/prices-confirm`, `/run-budget`, `/time-budget` |
| Integrations | `/mcp`, `/skills`, `/reload`, `/extension` |

Run `/help` for exact arguments. Commands that mutate durable state use review,
digest confirmation or explicit acknowledgement rather than applying an unseen
change immediately.

Manual `/compact` runs in the background while the terminal remains responsive.
The footer shows its progress; Ctrl+C requests cancellation. New turns and
session/model changes wait until the compaction worker has stopped. `/quit`
cancels an active compaction and exits after the worker returns.

Conversations are saved per-workspace, so quitting and re-running `hand` in
the same directory picks up where you left off — including across a
`/model` switch, so relaunching `hand` does not by itself give you a clean
slate.

> **After `/model`, a long-lived conversation can still "remember" the old
> model.** Hand tells the model which one it's running as fresh on every
> turn, but if the conversation already has several turns where the model
> stated its old identity (e.g. you asked "which model are you" a few times
> before switching), it can keep repeating that instead of the current,
> correct answer — a model generally treats what it already told you as
> stronger evidence than an instruction to disregard it. This isn't a bug
> in the switch itself: the request really is going to the new model, it's
> just the model's self-report that lags. `/new` starts a blank
> conversation with no such history to compete with, which reliably clears
> it up.

## Status line

The line just above the input box is always showing, whether or not a turn
is running. While idle, after a turn has completed at least once:

```
ready   anthropic/claude-sonnet-5  ·  ctx 38.4k/200k (19%)  ·  turn 6.1k tok  ·  session 21.9k tok  ·  last turn 5.8s
```

While a turn is in progress:

```
⠋ working... 4.2s · 3 tool calls   anthropic/claude-sonnet-5  ·  ctx 38.4k/200k (19%)  ·  turn ~1.2k tok  ·  session 21.9k tok
```

- **Run state** — `ready`, or a spinner with a live elapsed-time counter while
  a turn is in progress, plus a running count of tool calls made so far this
  turn (shown once at least one has run) — concrete evidence of progress on
  a turn that's mostly tool calls with no text in between, where the token
  figures below don't move at all until the turn finishes.
- **The active model** — the current `provider/model` string. Shown here,
  not just in the startup banner, because [`/model`](#slash-commands) can
  change it mid-session and the banner scrolls out of view — this is the
  one place that always reflects what's actually running right now.
- **`ctx`** — how much of the active model's context window the last
  *completed* turn's request used, and the window size itself (e.g.
  `38.4k/200k (19%)`). Turns red once usage crosses 85% of the window, a
  nudge that compaction (automatic, or `/compact`) is worth watching for.
- **`turn`** — the most recently completed turn's total tokens (input +
  output). While a turn is running, shows a live `~`-prefixed estimate of
  the in-progress response instead (token usage is only reported once a
  turn finishes, so this is approximate, not exact).
- **`session`** — the running total across every turn since this `hand`
  process started (resets on restart — it isn't persisted with the saved
  session).
- **`last turn`** — how long the most recently completed turn took (hidden
  while idle before any turn has completed, and while a turn is running —
  see the live elapsed-time counter in the run state instead).

`/usage` prints the same figures (plus the raw input/output/cache
breakdown) as a one-off transcript entry, if you want it in the scrollback.

Tool output shown under a `✓`/`✗` line is a short preview — 5 lines / 500
characters — not the full result. Scroll up (`pgup`/`pgdown`, `ctrl+u`/
`ctrl+d`) to review earlier output; the preview cap keeps the transcript
itself scannable rather than a full pager for every command.

Mouse scrolling is enabled by default. In iTerm2, hold Option while dragging
to select text, or disable **Report mouse clicks & drags** while keeping wheel
reporting enabled. `/mouse off` restores ordinary selection in other terminals.

## Search

Search results include a completeness footer. The `search` tool streams large
files and honours workspace/nested `.gitignore` rules; `include_ignored` and
`include_hidden` broaden that scope explicitly. Git metadata, binary content
and non-regular entries are excluded and counted. Read errors, lines exceeding
1 MiB, cancellation and result/output limits are reported explicitly. An
incomplete search cannot establish that a match is absent. Git and `rg` are
not required. The default output budget is 64 KiB, configurable up to 256 KiB.

## Execution isolation

Host mode is the default and is labelled as unrestricted host execution. To run
shell tools and compatible reviewed extensions inside a container, configure a
local Docker engine, a previously pulled immutable image, and the matching
`hand-tool-worker-linux` shipped in the release archive:

```json
"execution": {
  "backend": "container",
  "docker": "/absolute/path/to/docker",
  "socket": "/absolute/path/to/docker.sock",
  "image": "sha256:<64 hex digits>",
  "worker": "/absolute/path/to/hand-tool-worker-linux",
  "worker_sha256": "<digest from WORKER.json>",
  "writable": false,
  "network": false
}
```

The container receives only its declared workspace and declared resources. Writes
and network access are off unless enabled. Image and worker identities must be
immutable, paths must be absolute, and an unavailable backend is an error—Hand
does not retry the command on the host. `/boundary` reports the effective trust
boundary.

MCP servers and hooks are integration processes or remote services other than
the containerized tool worker. If either is configured with container mode,
startup fails until `trust_external_mcp` or `trust_external_hooks` is explicitly
set for the relevant boundary. These acknowledgements expose the boundary; they
do not move those integrations into the container or reduce their
operating-system authority.

## Approval prompts

Hand gates `bash`, `write_file`, `edit_file`, persistent `todo_write`, mutating
`skill_manage` operations, unknown tools, and tools from untrusted MCP servers. Each prompt reads
`Allow <tool>? [y]es / [a]lways / [n]o`:

- **[y]es** — run just this call
- **[a]lways** — never ask again for this tool in this workspace
    (persisted to `.hand/settings.json` in the workspace)
- **[n]o** — deny the call

If a workspace's `.hand/settings.json` already grants "always allow" for
something (e.g. it was committed to a repo you just cloned), Hand asks you
to confirm trusting it once per workspace before honoring those entries —
add `.hand/` to your `.gitignore` if you don't want your own approvals
checked into version control.

## Extending Hand

Hand has three, orthogonal ways to extend what it can do, and it's worth
being precise about what each one actually adds:

- **MCP servers** add new *tools* the model can call.
- **Skills** add new *knowledge* the model can pull into context on demand —
  no new tools, just a name/description index plus a way to fetch the body.
- **Hooks** add *lifecycle interception* — a chance to observe or block
  something that's about to happen, for every tool call (built-in, MCP, or
  skill-related) and every turn, not just gated ones.

They compose: an MCP server's tools are gated and hookable exactly like
Hand's own `bash`/`write_file`/`edit_file`; a `PreToolUse` hook's `matcher`
can target `mcp__<server>__<tool>` just as easily as `bash`.

### MCP servers

[MCP](https://modelcontextprotocol.io) (Model Context Protocol) servers are
external processes (or remote endpoints) that expose their own tools — a
GitHub server exposing `create_issue`, a database server exposing `query`,
and so on. Configure them in `~/.hand/config.json`'s `mcp_servers` list (see
[Configuration](#configuration)); there's no per-project MCP config.

Each entry is either:

- a **local command** (`command`, `args`, `env`) — Hand spawns it as a
  subprocess and speaks MCP over its stdin/stdout, or
- a **remote server** (`url`, `headers`) — Hand connects over HTTP.

Set exactly one of the two per entry.

**Connecting.** Every configured server is connected synchronously, before
the TUI (or one-shot output) appears — a slow server adds visible startup
delay, with `hand: connecting to N configured MCP server(s)...` printed
while it waits. Connecting is bounded to 15 seconds total across every
configured server; on timeout, Hand exits with an error rather than hanging
indefinitely (a server that finishes connecting a few seconds *after* the
timeout is still cleaned up properly in the background, so it won't leak a
subprocess).

**Naming and gating.** Each tool a connected server exposes gets registered
into Hand's tool registry as `mcp__<server-name>__<tool-name>` — visible to
the model under that full name. By default, every tool from every server is
gated exactly like `bash`/`write_file`/`edit_file`: it triggers the same
`Allow <tool>? [y]es / [a]lways / [n]o` approval prompt (see
[Approval prompts](#approval-prompts)) before it's allowed to run. Setting
`"trusted": true` on a server's config entry skips the prompt for every tool
that server exposes — only do this for a server whose output you trust as
much as Hand's own built-in tools, since an untrusted MCP server can expose
arbitrary tools you've never seen before that might use identical names to a
trusted one (Hand resolves the *longest* matching configured server name
first specifically to avoid a shorter trusted name accidentally covering a
longer untrusted one that happens to share a prefix).

```json
"mcp_servers": [
  { "name": "github", "command": "npx", "args": ["-y", "@modelcontextprotocol/server-github"], "env": { "GITHUB_TOKEN": "..." }, "trusted": false },
  { "name": "remote-example", "url": "https://example.com/mcp", "headers": { "Authorization": "Bearer ..." }, "trusted": true }
]
```

### Skills

Skills are reusable, on-demand-loaded procedural knowledge: a name and
one-line description show up in Hand's system prompt at startup as a
"## Skills" index, and the full body only loads into context when the agent
actually decides it needs it (via a `load_skill` tool call) — cheaper than
stuffing every skill's full content into the system prompt on every turn,
and the index itself is built once at startup and reused unchanged for the
life of the process.

**Where they live.** Hand reads two directories and merges them, project
winning on a name collision:

- `~/.hand/skills/` — personal skills you maintain by hand, shared across
  every project.
- `<workspace>/.hand/skills/` — project-specific skills. Committable to a
  repo alongside `HAND.md`/`AGENTS.md`, so a team can share "how we do
  things here" knowledge the same way they share project instructions.

**Format.** Each skill is its own directory containing `SKILL.md`, with a
frontmatter block for metadata and everything after it as the body the model
sees once loaded:

```
.hand/skills/run-tests/SKILL.md
```

```markdown
---
description: how to run this repo's test suite
---

Run `make test`. Integration tests additionally need a running Postgres —
see docker-compose.yml.
```

**How a skill actually gets used in a turn:** at startup, Hand scans both
directories, builds the merged index, and injects it into the system
prompt. Mid-conversation, if the model decides a skill looks relevant to
what you asked, it calls `load_skill` with the skill's name; Hand returns
the full `SKILL.md` body as that tool call's result, which then sits in
context for the rest of the conversation like any other tool output
(subject to the same compaction as everything else — see
`compaction_threshold` below).

**Self-authoring.** The model can also create, patch, replace, remove,
list, or get skills itself via a `skill_manage` tool — useful for "remember
how to do this" requests, where the agent writes its own procedural
knowledge for next time. Self-authored skills always land in the
**project-local** store (`<workspace>/.hand/skills/`), never your personal
`~/.hand/skills/`, so an agent-authored skill stays scoped to the repo it
was learned in rather than silently spreading across every project you use
Hand in. `load_skill` and the `skill_manage` get/list operations are read-only
and do not prompt. Create, patch, replace and remove require approval,
including in one-shot mode. The index refreshes for subsequent model requests
after self-authoring. Use `/reload` while idle to refresh external skill edits.

Run `/skills` to see what's currently loaded, from both directories.

Installed package skills can be selected explicitly with `--package-skills`.
Their `load_skill` results include source provenance and support bounded,
hash-verified reads of declared relative text resources. See
[installed package skills](docs/package-skills.md) for selection and usage.

### Hooks

Hooks are shell commands Hand runs at five points in the agent loop —
before a tool call, after a tool call, at session start, when you submit a
prompt, and when a turn stops — configured globally in
`~/.hand/config.json`'s `hooks` list (see [Configuration](#configuration)).
Unlike skills, there is no per-project hooks file: a hook executes an
arbitrary command, so it gets the same trust tier as an `mcp_servers` entry
(also global-only) rather than something a cloned repo could ship and have
run automatically the first time Hand touches it.

Every hook receives a versioned JSON object on **stdin**:

```json
{"version":1,"event":"PreToolUse","workspace":"/work/project","tool_name":"bash","tool_input":{"command":"go test ./..."}}
```

| Event | Fires | Can block? | JSON payload fields |
|-------|-------|------------|---------------------|
| `PreToolUse` | Before a tool call | Yes | `tool_name`, `tool_input` |
| `PostToolUse` | After a tool call | Observe-only | `tool_name`, `tool_input`, `tool_result`, `tool_error` |
| `SessionStart` | When a session begins | Observe-only | Common metadata |
| `UserPromptSubmit` | Before sending the user's message | Yes | `prompt` |
| `Stop` | After a turn ends | Yes — see [Goal loop](#goal-loop) | `stop_reason`, `goal_iteration` |

`tool_input` is a JSON object; prompt, output and error fields are strings.
Empty optional fields may be omitted. Small metadata environment variables
remain available: `HAND_HOOK_VERSION`, `HAND_HOOK_EVENT`, `HAND_WORKSPACE`,
`HAND_TOOL_NAME`, `HAND_STOP_REASON` and `HAND_GOAL_ITERATION` where applicable.

**Migration:** scripts reading `HAND_PROMPT`, `HAND_TOOL_INPUT`,
`HAND_TOOL_RESULT` or `HAND_TOOL_ERROR` should read the corresponding JSON
stdin fields instead. Temporarily setting `"legacy_env": true` exports these
fields too, but rejects any environment value larger than 16 KiB. It never
silently truncates them. The default stdin protocol supports large payloads
without relying on operating-system argument/environment limits.

**Ordering relative to gating.** `PreToolUse` hooks run for *every* tool
call, not just gated ones — `read_file` and `web_search` trigger matching
hooks too, even though neither ever shows an approval prompt. For a gated
tool, a `PreToolUse` hook runs *before* the interactive approval prompt, so
a hook can auto-deny a call before you're ever asked about it.

**Exit codes and failure policies:**

- Exit `0` allows the operation or passes the validator.
- Exit `2` denies `PreToolUse`, aborts `UserPromptSubmit`, or requests another
  goal iteration for `Stop`. Stop uses stdout, then stderr, as its next prompt.
- Other nonzero exits, spawn errors, timeouts and excessive output fail
  validators closed by default (`"failure_policy": "deny"`). A failed Stop
  validator returns an error, so one-shot mode cannot report success.
- Optional observers may set `"failure_policy": "warn"` to log failures and
  continue. This is the default for observe-only `PostToolUse` and
  `SessionStart`; these events reject `"deny"` because they cannot veto work.
  Cancellation stops validation even with `"warn"`.

Timeout defaults to 30 seconds (`timeout_seconds` overrides it). stdout and
stderr capture is limited to 64 KiB each while the process runs; exceeding
that limit is a validation failure. On macOS and Linux, cancellation joins the
hook's process group and bounded pipe draining before the run settles.

Existing validator configurations now fail closed on script errors. To retain
legacy warning-only behaviour for an optional script, explicitly select
`"failure_policy": "warn"`. Use a blocking event for mandatory validation.

A `matcher` (exact tool name, or `"*"`/omitted for every tool) restricts a
`PreToolUse`/`PostToolUse` hook to specific tools — including MCP ones, e.g.
`"matcher": "mcp__github__create_issue"`. `SessionStart`/`UserPromptSubmit`/
`Stop` have no tool to match against, so `matcher` is ignored for them.

Two worked examples:

```json
{ "event": "PreToolUse", "matcher": "bash", "command": "./scripts/check-command.sh" }
```

`check-command.sh` reads `tool_input` from its JSON stdin payload
(containing the shell command), and exits `2` with a reason on stderr to
block anything it doesn't like — e.g. a company policy against `curl | sh`.

```json
{ "event": "Stop", "command": "notify-send", "args": ["hand finished"] }
```

A simple desktop notification every time a turn ends, regardless of how it
ended (`HAND_STOP_REASON` is one of `completed`, `max_turns`, `error`, or
`aborted`) — no blocking, just an observation.

### Goal loop

A `Stop` hook can do more than notify — it can tell Hand the goal hasn't
been met yet, and Hand will automatically run another turn to keep working,
without you re-typing anything. This is what makes an unattended "keep
fixing until `make test` is green" run possible, in both `-p` one-shot and
the interactive TUI.

It reuses the exact same exit-code contract as everything else: your `Stop`
hook exits `0` when the goal is met (stop as normal — the ordinary,
backward-compatible case, like the `notify-send` example above), or exits
`2` when it isn't, printing what's still wrong to **stdout** (falls back to
stderr, then to a fixed message if both are empty) — that text becomes the
next prompt Hand runs automatically:

```json
{
  "event": "Stop",
  "command": "sh",
  "args": ["-c", "if make test; then exit 0; else exit 2; fi"]
}
```

A rough sketch: exit `0` if `make test` passes, otherwise print the
failure output and exit `2` — Hand then runs another turn with that output
as the prompt, and repeats until the tests pass or the loop gives up.

The TUI displays one final result for each user request, including its automatic
continuations. It distinguishes completion, verification failure, exhausted
limits, cancellation and execution failure. A completed answer explicitly
states whether configured mandatory checks passed. The final allowed iteration
still runs validation: it may succeed at the cap, but a request for another
iteration reports a limit failure.

In the interactive TUI, an auto-continued turn shows up with a distinct
`↻ continuing (goal loop N/max): ...` transcript line, never rendered like
something you typed. Pressing ctrl+c during an auto-continued turn stops
the loop as well as the turn — it does not immediately try again.

**Safety cap.** `max_goal_iterations` (config.json) / `--max-iterations`
bounds how many turns a single goal-loop chain may run automatically
(default `10`, the first turn plus up to 9 continuations) before Hand gives
up regardless of what the hook says — the backstop against a broken or
malicious goal-check script looping forever and burning API tokens with no
one approving the overall goal each time. This is a *different* knob from
`max_turns`, which bounds the tool-use loop *within* one turn, not how many
turns get chained.

**What's unaffected.** Per-tool-call approval gating inside each turn is
completely unchanged — `bash`/`write_file`/`edit_file` and untrusted MCP
tools still prompt (or run their own `PreToolUse` hooks) exactly as before.
The goal loop only decides whether to start *another* turn once one ends;
it has no effect on what happens *inside* a turn. An errored turn
(`EventError`) never triggers a continuation, regardless of what a `Stop`
hook would have said.

## Configuration

Hand reads `~/.hand/config.json` on startup (created with defaults on first
run):

```json
{
  "model": "anthropic/claude-sonnet-5",
  "default_profile": "primary",
  "profiles": {
    "primary": {
      "provider": "anthropic",
      "model": "claude-sonnet-5",
      "credential_env": "ANTHROPIC_API_KEY",
      "input_types": ["text", "image"],
      "context_limit": 200000,
      "max_output": 64000,
      "reasoning": "medium",
      "reasoning_levels": ["off", "low", "medium", "high"]
    }
  },
  "base_url": "",
  "max_turns": 50,
  "max_goal_iterations": 10,
  "fallback_model": "",
  "markdown_style": "dark",
  "compaction_threshold": 0.4,
  "execution": {
    "backend": "host"
  },
  "mcp_servers": [
    {
      "name": "github",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": { "GITHUB_TOKEN": "..." },
      "trusted": false
    },
    {
      "name": "remote-example",
      "url": "https://example.com/mcp",
      "headers": { "Authorization": "Bearer ..." },
      "trusted": true
    }
  ],
  "hooks": [
    {
      "event": "PreToolUse",
      "matcher": "bash",
      "command": "./scripts/check-command.sh",
      "timeout_seconds": 10
    },
    {
      "event": "Stop",
      "command": "notify-send",
      "args": ["hand finished"]
    }
  ]
}
```

- `model` / `base_url` / `max_turns` / `max_goal_iterations` /
  `fallback_model` / `markdown_style` / `compaction_threshold` are the same
  values the CLI flags above override for a single run. `markdown_style`
  accepts `dark`, `light`, `ascii`, `notty`, `pink`, `dracula`, or
  `tokyo-night`; anything else (including glamour's own `auto` —
  deliberately not offered, since it detects light/dark by querying the
  terminal in a way that can corrupt the input box) falls back to `dark`.
  `max_goal_iterations` is unrelated to `max_turns` — see
  [Goal loop](#goal-loop).
- `default_profile` selects one entry from `profiles`. Profiles bind the
  provider, bare model ID, credential environment-variable name, endpoint and
  declared capabilities. Hand never stores the credential value. Explicit
  context and output limits win over verified catalogue/server metadata;
  unknown limits use a labelled conservative fallback rather than an
  advertised maximum being guessed as active.
- `compaction_threshold` controls how eagerly Hand summarizes older
  conversation to keep token usage down — preventive compaction fires once
  the estimated context passes this fraction of the model's context window
  (default `0.4`, i.e. 40%). A tool-call-heavy turn (many file reads, test
  runs, greps) can otherwise re-send a large tool result on every subsequent
  call in that same turn until it's compacted away, so lower is more
  aggressive about controlling token spend at the cost of summarizing sooner
  (and losing a bit of verbatim detail from earlier in the conversation);
  higher keeps more raw context around longer. Must be in `(0, 1]` —
  anything else falls back to the default.
- `mcp_servers` — see [MCP servers](#mcp-servers) above for what each field
  means and how gating/trust works.
- `hooks` — see [Hooks](#hooks) above for the five events, the exit-code
  contract, and what each `HAND_*` environment variable carries.
- `execution` — see [Execution isolation](#execution-isolation). `host` is
  unrestricted; `container` is fail-closed and requires pinned runtime inputs.

## Development

```sh
make build   # build bin/hand
make run     # go run ./cmd/hand, no build step
make test    # go test ./...
make vet     # go vet ./...
make fmt     # go fmt ./...
make install # go install ./cmd/hand
```

For the offline CI checks, run `python3 scripts/validate.py --output /tmp/hand-validation-001`
with a new output directory. It inventories tests, checks formatting/build/vet,
runs uncached race-enabled tests with coverage, rejects skipped or missing tests,
and checks `--help`/`--version` without credentials. Raw logs and a JSON report
remain in the output directory, including on failure. The v0.3.0 release
candidate was also qualified with the retained-scope checker and isolated-agent
gateway probes described in [`docs/acceptance/`](docs/acceptance/).

`hand --version` reports the version and source commit without loading configuration.
The Go binary needs no separate language runtime; shell execution requires Bash
on macOS/Linux, and optional MCP servers/extensions may require their own runtimes.

## Releasing

```sh
make dist                      # cross-compile darwin/linux amd64/arm64 into dist/*.tar.gz + SHA256SUMS
make release VERSION=v0.3.0    # tag and push — the step below then runs automatically
```

Pushing a `v*` tag triggers
[`.github/workflows/release.yml`](.github/workflows/release.yml): it first requires
the shared Linux/macOS validation workflow to pass for that commit, then runs
`make dist`, verifies checksums and archive contents, smoke-tests the native binary,
and publishes a GitHub release with the
resulting archives and checksums attached, via
[`softprops/action-gh-release`](https://github.com/softprops/action-gh-release).
The distribution directory must be new; use `DIST_DIR` to retain prior builds.
Every archive includes the Linux worker and its digest manifest, and CI requires
worker validation. Publish only a reviewed commit whose branch validation is
green; the tag workflow repeats validation before creating the release.

## License

MIT — see [`LICENSE`](./LICENSE).

Permission migration, revocation and rejected-journal recovery are described in
[the permission migration guide](docs/permission-migration.md).

Session navigation, checkpoint previews, interrupted restore recovery and named
verification are described in [sessions and restore](docs/sessions-and-restore.md).

For subprocess or embedded integrations, see [automation and the Go SDK](docs/automation-and-sdk.md).

See [packages and extensions](docs/extensions.md) for installation, activation, updates and rollback.

For common errors and recovery steps, see [troubleshooting](docs/troubleshooting.md).

### Persistent Bash approval

To allow all Bash commands in the current project, enter this while Hand is idle:

```text
/permissions allow bash --project
```

The grant persists across restarts for the current configuration. Inspect it with
`/permissions` and remove it with `/permissions revoke project-bash`. This is a
Bash approval preference; execution boundaries still come from the configured
backend. The approval prompt's **always this command** choice only remembers the
exact command being shown.
