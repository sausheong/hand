# Extension RPC controls (development)

When the application creator configures `Controller.Extensions`, `hello` advertises
these methods. Configuring the adapter does not discover, install or approve any
project extension. Activation is explicitly configured; terminal usage is described below.

| Method | Parameters | Result |
| --- | --- | --- |
| `extension.command` | `name`, `command`, `arguments` strings | Durable asynchronous request record |
| `extension.questions` | Empty object | Pending questions with host token, extension identity and structured question |
| `extension.answer` | `token`, `answer` object | `answered: true` after validating the live question |

An answer contains the question `id` and either `text`, an option `choice`, or
`cancelled: true`. Use the host token returned by the pending query. It identifies
one live question; a reused extension question ID alone is insufficient.

Commands execute outside the dispatcher lock, so question polling and answers
remain available. Poll `request.get` with the original command request ID for its
stored terminal response. Successful commands contain validated presentation
blocks. Errors are terminal error responses. The existing `cancel` method cancels
the command; dispatcher disconnect cancels and joins it before shutdown returns.
The application creator closes the extension host before closing its session.

Use stable unique request IDs. Replaying an identical command returns its stored
record without re-execution. Replaying the same answer request returns the stored
response; a new request attempting to reuse an expired token is rejected.
Interrupted durable operations retain the ledger's uncertain-state semantics and
are never automatically repeated. Answers do not grant resource permissions.

The permanent `TestExtensionRPCQuestionReplayAndDisconnect` test launches the real
Go task-note peer and exercises answer, cancel and disconnect paths. Admission is
test-owned; this does not qualify product trust prompts, a native terminal journey
or the final release candidate.

## Explicit startup

The built CLI accepts `--rpc --extension-config /absolute/review.json
--approve-extension-config SHA256`. Supply the lowercase SHA-256 of the complete
review file after reviewing its contents. This explicitly approves unrestricted
host extension execution for that file; no project directory is searched for
extensions. File changes require a new approval. Execution also revalidates each
reviewed executable and resource before launch.

The version-1 file contains `snapshot_root` (private and outside the workspace),
`identities` (extension name to installed package identity), and `reviews` (the
LaunchReview objects produced by `extensions.ReviewHostLaunch`). Each review must
match the current canonical workspace. Configuration is limited to 256 KiB and
16 peers, with duplicate/unknown JSON fields rejected. Generate reviews with the command below; do not invent file hashes or omit
reviewed runtime and environment details. Host runtime libraries remain outside
the source snapshot.

Startup rejects this host factory if an isolation backend is selected; it does
not fall back to host execution. Startup supports RPC and interactive modes. The built-CLI RPC journey is reproducible below.

## Generate a review without execution

Create an input file using absolute paths, for example:

```json
{
  "version": 1,
  "snapshot_root": "/absolute/private-extension-snapshots",
  "extensions": [{
    "identity": "examples/task-note",
    "launch": {
      "name": "task-note",
      "executable": "/absolute/hand-task-note",
      "workspace": "/absolute/workspace",
      "capabilities": ["commands", "questions", "state", "context.transform"]
    }
  }]
}
```

Run `hand --review-extensions /absolute/input.json --extension-review-output
/absolute/review.json`. This reads and fingerprints the declared files without
starting extensions or a provider. The output must not already exist. Stdout
contains the file SHA-256; stderr reminds you to inspect the generated review.
Reviewing does not grant execution permission. After inspection, supply that
digest separately with `--approve-extension-config` during RPC startup.

For Python, set `executable` to the absolute Python 3 executable, `package_dir`
to the example directory, `files` to `["main.py"]`, and `arguments` to
`["-I", "-B", "${package}/main.py"]`. Only explicitly listed package files are
snapshotted. The generated review discloses arguments, environment, capabilities,
workspace, resource hashes and the unrestricted host execution boundary.


## Reproduce the built-CLI journey

Build Hand and the Go task-note example, then run:

```sh
python3 scripts/check_extension_rpc.py \
  --hand-binary /absolute/hand \
  --go-extension /absolute/hand-task-note \
  --python-binary /absolute/python3 \
  --python-source /absolute/examples/extensions/python-task-note/main.py \
  --out /absolute/new-evidence-directory
```

The runner generates and approves only its isolated fixture reviews. It uses a
non-listening local provider endpoint and sends no model prompts. It checks both
languages, wrong approval rejection, question/answer flow, request replay after
restart, cancellation and snapshot cleanup. Results include binary and runner
hashes and raw client transcripts. This is development evidence until rerun on
the exact frozen candidate under the full acceptance contract.

## Interactive terminal

The same explicit configuration and approval flags now support interactive mode
(omit `--rpc`). Run `/extension task-note note` to invoke the example. While its
question is visible, Enter submits free text; use `choice:ID` for an option when
free text is allowed, or the bare option ID for a choice-only question. Esc sends
a cancelled answer; Ctrl+C cancels the command. Question responses grant no tool
permissions. Extension output uses sanitised declarative transcript content.

The terminal model integration is covered by a real peer test. A native macOS
PTY journey also passed; richer code/list presentation and final platform
qualification remain outstanding.


Reproduce the native terminal journey with:

```sh
python3 scripts/check_extension_terminal.py \
  --hand-binary /absolute/hand \
  --go-extension /absolute/hand-task-note \
  --out /absolute/new-terminal-evidence
```

It checks free-text entry, Esc question cancellation, Ctrl+C command cancellation,
continued core usability, the session journal and snapshot removal. The runner
records terminal bytes and binary/peer hashes. It uses an isolated fixture review
and sends no model prompts.

## Reload reviewed configuration

Generate a fresh review and inspect it before reloading. In the terminal use
`/reload REVIEW_FILE APPROVED_SHA256`; plain `/reload` still refreshes skills.
The RPC method `extension.reload` takes `path` and `digest` and returns an
asynchronous durable request record. Poll `request.get` for a result containing
`reload`, `completed`, and an `error` when applicable. Inspect `reload.committed`:
a retirement cleanup error can occur after the new registry has committed.

Reload stages changed peers before committing the new factory and registry.
Unchanged healthy peers are reused; failed staging retains the working peers
and previous factory. Commands must finish or be cancelled before reload.
Package identity cannot change for an already-seen extension name during the
session. Use a distinct name for a different package. At most 256 names may be
seen in one host lifetime. A reviewed empty `extensions` input creates a review
with no peers; applying it removes all active peers and joins their cleanup.
No startup side effects outside the managed registry can be rolled back.

The RPC reproduction runner now also checks unchanged-peer reuse, failed staged
handshake preservation, recovery after cancellation, removal of all peers and
rejection of commands for a removed peer. The terminal runner reloads after Ctrl+C
and invokes the recovered peer before checking the journal and shutdown. Both
journeys passed on the same macOS development binary; see
`extension-reload-journeys.json` for hashes and raw evidence.

Extension questions also appear during model runs and queued runs. The terminal
keeps any existing follow-up draft aside while the question is active and restores
it afterward. Polling is tied to the active run generation so stale messages do
not recreate an old question. Enter/Esc answer only the current live question;
Ctrl+C still cancels the owning operation. These controls do not grant tool
permissions. The current extension request timeout still bounds the answer wait.

Extension activation is a one-time controller startup operation. It holds shared operation ownership until peers and runtime hooks are attached; it refuses overlap with active work. After successful activation, use reviewed `extension.reload` or `/reload REVIEW_FILE SHA256` for reconfiguration. Closing the host does not permit another activation on that controller, because runtime hooks retain its lifetime binding; create a new controller/application lifetime when restarting.

Interactive command results retain declarative text, code and list blocks in the transcript. Hand adds extension attribution, neutral code borders with language labels, and hanging list indentation. Blocks reflow when the terminal width changes. Text and code are literal rather than interpreted as Markdown; content cannot supply terminal styles, hyperlinks, approval states or execution outcomes. The RPC presentation schema remains unchanged.

Commands and `run.start` notifications have a five-minute ceiling for the entire operation, including all user questions; another question does not restart that clock. Other requests use a 30-second ceiling. Earlier caller/run deadlines and explicit cancellation still take precedence. `run.finish` is a terminal observer with the runtime's shorter two-second budget and should not ask interactive questions. A timed-out admitted request terminates its peer and joins callbacks; use reviewed reload to recover it before another command.
