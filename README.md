# Hand

Hand is a minimal, extensible terminal coding agent. Point it at a directory
and it can read, search, edit, and run commands in that workspace on your
behalf — with a lightweight approval gate for anything that changes files or
runs shell commands.

Hand is deliberately small: one binary, one config file, no daemon, no
project scaffolding forced onto your repo. It's built on top of
[`harness`](https://github.com/sausheong/harness), a Go library for building
LLM agents.

## Installing

Hand ships as a single static binary — no runtime dependencies.

### Build from source

You need [Go 1.25+](https://go.dev/dl/).

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

Every [release](https://github.com/sausheong/hand/releases) ships a
`hand-<version>-<os>-<arch>.tar.gz` archive for each platform —
`darwin-amd64`, `darwin-arm64`, `linux-amd64`, `linux-arm64` — plus a
`SHA256SUMS` file, all built and published automatically by CI when the
release is tagged (see [Releasing](#releasing) below). Grab the one
matching your platform:

```sh
curl -LO https://github.com/sausheong/hand/releases/latest/download/hand-<version>-<os>-<arch>.tar.gz
tar -xzf hand-<version>-<os>-<arch>.tar.gz
mv hand-<version>-<os>-<arch>/hand /usr/local/bin/hand   # anywhere on your PATH
```

To verify the download against `SHA256SUMS` (also attached to the release):

```sh
curl -LO https://github.com/sausheong/hand/releases/latest/download/SHA256SUMS
shasum -a 256 -c SHA256SUMS --ignore-missing   # or: sha256sum -c ... on Linux
```

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
| Ollama      | none — local servers don't authenticate requests |

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

**Ollama** runs models locally — no key, no network round-trip beyond your
own machine. It needs no other setup either: `--model ollama/<model-name>`
(e.g. `ollama/qwen2.5:3b`, matching what `ollama list` shows) talks to
`http://localhost:11434/v1` by default; pass `--base-url` if your server
runs elsewhere. This also covers any other local server sharing Ollama's
OpenAI-compatible endpoint shape (LM Studio, llama.cpp's server, vLLM).

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

### Flags

| Flag               | Description |
|---------------------|-------------|
| `--model`           | `provider/model` to use for this run, e.g. `anthropic/claude-sonnet-5` — overrides `~/.hand/config.json` |
| `--base-url`        | Custom API base URL — required for `litellm`, optional for `openai`/`openrouter`, not supported for `gemini` |
| `--max-turns`       | Cap the agent's tool-use loop for this run (default: 50, or `max_turns` in config) |
| `--fallback-model`  | `provider/model` to retry against on a transient provider error, same provider as `--model` |
| `--markdown-style`  | Glamour style for rendering assistant Markdown: `dark`, `light`, `ascii`, `notty`, `pink`, `dracula`, `tokyo-night` (default `dark`) — overrides `~/.hand/config.json` |
| `--compaction-threshold` | Fraction (0-1] of the context window that triggers preventive compaction (default `0.4`) — overrides `~/.hand/config.json` |
| `--max-iterations`  | Cap how many turns a [Stop-hook goal loop](#goal-loop) may chain automatically (default `10`, or `max_goal_iterations` in config) |
| `--new-session`     | Discard this workspace's saved session and start fresh |
| `-p "<prompt>"`     | Run one turn non-interactively and exit (no TUI) |
| `--yes`             | Auto-approve all gated tool calls for this run (only valid with `-p`) |

## Slash commands

Inside the interactive UI, a message starting with `/` runs a command
instead of being sent to the model. Typing `/` shows an auto-complete
dropdown (arrow keys to move, Tab or Enter to fill it in).

| Command          | Description |
|-------------------|-------------|
| `/help`           | Show the command list |
| `/model`          | Show the active model, or `/model <provider/model>` to switch |
| `/new`            | Discard this workspace's saved session and start fresh |
| `/clear`          | Clear the on-screen transcript (the saved session is untouched) |
| `/compact`        | Force a context-compaction pass now |
| `/usage`          | Show token usage: this turn, session total, and context window |
| `/skills`         | List available skills (personal + project) |
| `/exit`           | Quit Hand (`/quit` also works) |

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
ready   ctx 38.4k/200k (19%)  ·  turn 6.1k tok  ·  session 21.9k tok  ·  last turn 5.8s
```

While a turn is in progress:

```
⠋ working... 4.2s · 3 tool calls   ctx 38.4k/200k (19%)  ·  turn ~1.2k tok  ·  session 21.9k tok
```

- **Run state** — `ready`, or a spinner with a live elapsed-time counter while
  a turn is in progress, plus a running count of tool calls made so far this
  turn (shown once at least one has run) — concrete evidence of progress on
  a turn that's mostly tool calls with no text in between, where the token
  figures below don't move at all until the turn finishes.
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

Hand doesn't capture the mouse, so your terminal's normal text
selection/copy works exactly as it would anywhere else.

## Approval prompts

Hand gates anything that isn't read-only: `bash`, `write_file`, `edit_file`,
and any tool from an untrusted MCP server. Each prompt reads
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
Hand in. Note that `load_skill`/`skill_manage` are **not** gated — no
approval prompt — since they're confined to the skills directory rather
than able to touch arbitrary files, unlike `write_file`. A skill created or
edited mid-session shows up in the system prompt starting the *next* run,
not immediately, since the index is fixed at startup.

Run `/skills` to see what's currently loaded, from both directories.

### Hooks

Hooks are shell commands Hand runs at five points in the agent loop —
before a tool call, after a tool call, at session start, when you submit a
prompt, and when a turn stops — configured globally in
`~/.hand/config.json`'s `hooks` list (see [Configuration](#configuration)).
Unlike skills, there is no per-project hooks file: a hook executes an
arbitrary command, so it gets the same trust tier as an `mcp_servers` entry
(also global-only) rather than something a cloned repo could ship and have
run automatically the first time Hand touches it.

**The five events:**

| Event | Fires | Can block? | Extra env vars |
|-------|-------|------------|-----------------|
| `PreToolUse` | Before any tool call — built-in, MCP, or skill-related | Yes | `HAND_TOOL_NAME`, `HAND_TOOL_INPUT` |
| `PostToolUse` | After a tool call returns (success or error) | No (observe-only) | `HAND_TOOL_NAME`, `HAND_TOOL_INPUT`, `HAND_TOOL_RESULT`, `HAND_TOOL_ERROR` (if any) |
| `SessionStart` | Once, when a session begins | No | — |
| `UserPromptSubmit` | Once per turn, before your message is sent | Yes | `HAND_PROMPT` |
| `Stop` | Once, when a turn ends (any outcome) | Yes — see [Goal loop](#goal-loop) | `HAND_STOP_REASON`, `HAND_GOAL_ITERATION` |

Every hook also receives `HAND_HOOK_EVENT` (the event name) and
`HAND_WORKSPACE`, plus the rest of Hand's own process environment.

**Ordering relative to gating.** `PreToolUse` hooks run for *every* tool
call, not just gated ones — `read_file` and `web_search` trigger matching
hooks too, even though neither ever shows an approval prompt. For a gated
tool, a `PreToolUse` hook runs *before* the interactive approval prompt, so
a hook can auto-deny a call before you're ever asked about it.

**The exit-code contract** (deliberately the simplest form that works —
matches the convention from Claude Code's own hooks, so anyone already
familiar with it can write one for Hand without learning a new contract):

- **Exit `0`** — allow. For `PostToolUse`/`SessionStart` this is the only
  outcome that matters; they can't block regardless of exit code.
- **Exit `2`** — deny (`PreToolUse`, with stderr shown as the approval
  denial reason), abort the turn (`UserPromptSubmit`, with stderr as the
  error), or keep going (`Stop` — see [Goal loop](#goal-loop) below, its
  own variant of this same convention). Nothing else happens after the
  first hook that returns this.
- **Anything else, or a timeout** (default 30s, `timeout_seconds` to
  change) — *fails open*: the call proceeds (or, for `Stop`, the turn
  simply ends) and a warning is logged, rather than a typo or a crashing
  script silently blocking every tool call or looping forever.

A `matcher` (exact tool name, or `"*"`/omitted for every tool) restricts a
`PreToolUse`/`PostToolUse` hook to specific tools — including MCP ones, e.g.
`"matcher": "mcp__github__create_issue"`. `SessionStart`/`UserPromptSubmit`/
`Stop` have no tool to match against, so `matcher` is ignored for them.

Two worked examples:

```json
{ "event": "PreToolUse", "matcher": "bash", "command": "./scripts/check-command.sh" }
```

`check-command.sh` reads `$HAND_TOOL_INPUT` (the raw JSON tool input,
containing the shell command), and exits `2` with a reason on stderr to
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
  "args": ["-c", "make test 2>&1 | tail -20 && exit 0 || (make test 2>&1 | tail -20; exit 2)"]
}
```

A rough sketch: exit `0` if `make test` passes, otherwise print the
failure output and exit `2` — Hand then runs another turn with that output
as the prompt, and repeats until the tests pass or the loop gives up.

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
  "base_url": "",
  "max_turns": 50,
  "max_goal_iterations": 10,
  "fallback_model": "",
  "markdown_style": "dark",
  "compaction_threshold": 0.4,
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

## Development

```sh
make build   # build bin/hand
make run     # go run ./cmd/hand, no build step
make test    # go test ./...
make vet     # go vet ./...
make fmt     # go fmt ./...
make install # go install ./cmd/hand
```

## Releasing

```sh
make dist                      # cross-compile darwin/linux amd64/arm64 into dist/*.tar.gz + SHA256SUMS
make release VERSION=v0.1.1    # tag and push — the step below then runs automatically
```

Pushing a `v*` tag triggers
[`.github/workflows/release.yml`](.github/workflows/release.yml): it runs
`make dist` on a clean runner and publishes a GitHub release with the
resulting archives and checksums attached, via
[`softprops/action-gh-release`](https://github.com/softprops/action-gh-release).
Nothing needs to be built or uploaded by hand beyond `make release` itself.
