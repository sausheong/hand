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

If you'd rather skip building it yourself, grab the binary for your platform
from the project's [Releases page](https://github.com/sausheong/hand/releases),
then:

```sh
chmod +x hand-<os>-<arch>
mv hand-<os>-<arch> /usr/local/bin/hand   # anywhere on your PATH
```

## Setting up an API key

Hand doesn't store API keys — each provider reads its key from its own
environment variable:

| Provider    | Environment variable  |
|-------------|------------------------|
| Anthropic   | `ANTHROPIC_API_KEY`    |
| OpenAI      | `OPENAI_API_KEY`       |
| Gemini      | `GEMINI_API_KEY`       |
| Qwen        | `DASHSCOPE_API_KEY`    |

Export the one matching your default model before running Hand, e.g.:

```sh
export ANTHROPIC_API_KEY=sk-ant-...
```

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
`**`/`#`/`` ` `` characters.

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
| `--base-url`        | Custom API base URL (e.g. a LiteLLM proxy) — not supported for Gemini |
| `--max-turns`       | Cap the agent's tool-use loop for this run (default: 50, or `max_turns` in config) |
| `--fallback-model`  | `provider/model` to retry against on a transient provider error, same provider as `--model` |
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
| `/usage`          | Show token usage from the most recent turn |
| `/exit`           | Quit Hand (`/quit` also works) |

Conversations are saved per-workspace, so quitting and re-running `hand` in
the same directory picks up where you left off.

## Status line

The line just above the input box is always showing, whether or not a turn
is running:

```
⠋ working... 4.2s   ctx 38.4k/200k (19%)  ·  turn 6.1k tok  ·  session 21.9k tok  ·  last turn 5.8s
```

- **Run state** — `ready`, or a spinner with a live elapsed-time counter
  while a turn is in progress.
- **`ctx`** — how much of the active model's context window the last turn's
  request used, and the window size itself (e.g. `38.4k/200k (19%)`).
- **`turn`** — total tokens (input + output) the most recently completed
  turn cost.
- **`session`** — the running total across every turn since this `hand`
  process started (resets on restart — it isn't persisted with the saved
  session).
- **`last turn`** — how long the most recently completed turn took.

`/usage` prints the same figures (plus the raw input/output/cache
breakdown) as a one-off transcript entry, if you want it in the scrollback.

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

## Configuration

Hand reads `~/.hand/config.json` on startup (created with defaults on first
run):

```json
{
  "model": "anthropic/claude-sonnet-5",
  "base_url": "",
  "max_turns": 50,
  "fallback_model": "",
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
  ]
}
```

- `model` / `base_url` / `max_turns` / `fallback_model` are the same values
  the CLI flags above override for a single run.
- `mcp_servers` lists [MCP](https://modelcontextprotocol.io) servers to
  connect at startup, extending Hand's built-in tools. Each entry is either
  a local command (`command`/`args`/`env`) or a remote server (`url`/
  `headers`) — set exactly one.
- `trusted: true` skips the approval gate for that server's tools. Leave it
  `false` (the default) unless you trust the server's output as much as
  Hand's own built-in tools.

## Development

```sh
make build   # build bin/hand
make run     # go run ./cmd/hand, no build step
make test    # go test ./...
make vet     # go vet ./...
make fmt     # go fmt ./...
make install # go install ./cmd/hand
```
