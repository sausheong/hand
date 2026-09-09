# Context inspection — development integration

Use `/context` while idle, or negotiate RPC `hello` and send `context.inspect`
with `{}` parameters. Inspection uses the current installed prompt and effective
history; it does not refresh discovery or invoke a model. Use `/reload` or a fresh
`skills.reload` request ID when explicitly refreshing skills.

The report includes estimated tokens and item counts for:

- Installed static prompt: identity, project guidance, skill index and memory.
- Effective conversation messages after compaction.
- Effective tool results.
- Registered tool schemas.

These are UTF-8 byte-based estimates with message framing and a fixed image
allowance. They are not provider-reported usage. Component rounding means the
sum can differ slightly from the total. The report excludes the next user input,
per-request date/identity suffix, and dynamic retrieval. Raw archived history is
not counted as active context.

Instruction paths and skill-index paths are captured with the installed prompt.
Their estimates are subsets of the static category, not additional tokens. The
view lists at most 128 sources and reports any omitted count. Editing files does
not retroactively change installed source metadata; skill reload updates its
index and source snapshot together.

Nested tool-result source detail, large-result detail, pins and compaction
controls remain under implementation. This grouped report alone does
not close M6.2 acceptance.

Run the built CLI regression with:

```sh
python3 scripts/check_context_rpc.py \
  --hand-binary /absolute/path/to/hand \
  --out /absolute/path/to/new-evidence-directory
```

The fixture verifies strict parameters, estimate disclosure, unchanged inspection
before reload, added skill contribution, duplicate reload replay, removal after a
fresh reload, and clean CLI exit. It requests no model runs. The output preserves
RPC transcripts, fixture state, binary/runner hashes and measured reports.

## Pinned objectives and constraints

Use `/pin ID objective TEXT` or `/pin ID constraint TEXT` to add or replace a
pin, `/pins` to list, and `/unpin ID` to remove. IDs use lowercase letters,
digits, hyphens and underscores (1–64 characters). At most 16 pins are allowed,
each with at most 2048 UTF-8 bytes of text.

Pins are session-wide across branches. They persist through compaction and
restart and are reinserted independently of the generated summary. Selecting a
different session selects its pins. Pins remain subject to the current user
request and do not grant tool permissions.

RPC equivalents are `context.pins` with `{}`, `context.pin` with `id`, `kind`
and `text`, and `context.unpin` with `id`. Mutation request IDs replay their
original outcome. Use a new ID for another change. Built-client pin qualification
and separate summariser controls remain pending.

## Summarisation failure

If the summariser cannot produce a usable summary, compaction is skipped and the
effective conversation, raw history and selected leaf are preserved. No placeholder
summary replaces them. Repeated failures open the existing automatic-compaction
circuit breaker; this does not make an oversized request fit the model. The
failure remains visible and may require changing the summariser configuration or
reducing the requested context through an explicit user action.

## Compaction focus

`/compact FOCUS TEXT` supplies one-off summarisation instructions (at most 4096
UTF-8 bytes). Current pins also reach the summariser. The focus applies to this
manual invocation only; it is not saved as a persistent session preference and
does not leak into subsequent automatic compaction. Pins remain independently
injected into model requests even if a summary omits them.

## Separate summariser review

`/summarizer PROFILE [OUTPUT_TOKENS TIMEOUT_SECONDS]` reviews a named profile.
It shows the model, destination, credential environment reference and per-attempt
limits, and explains that session history and pins go to that destination.
`/summarizer-confirm DIGEST` applies the displayed review once. `/summarizer`
shows the current independent selection or available profiles.

RPC provides `summarizer.review` with profile/limit fields, `summarizer.select`
with `options`, `digest` and `confirmed: true`, and `summarizer.status` with `{}`.
Selection uses a durable request ID for replay, but the selected provider is
currently process-local. A historical successful response does not prove a new
process has restored that selection; inspect current status. Startup persistence remains under implementation.

The default limits are 4096 output tokens and 60 seconds per attempt, configurable
to 1–32768 tokens and 1–300 seconds. These are not aggregate cost limits; retry
usage is additional. Changing the main model/profile preserves an independent
summariser selection.

`/summarizer-follow` reviews returning to the current main-model destination with
default summariser limits; confirm its digest as usual. RPC can review/select
`{"follow_main": true}` with optional output/timeout limits and no profile name.
A main-model change invalidates an unconfirmed review. Once confirmed, later main
model changes also change the summariser, while its reviewed limits remain.
Status reports the effective following model/destination and `independent: false`.

### Reapply a reviewed summariser selection at startup

For interactive or RPC use, pass `--summarizer-config /absolute/path/summary.json`.
Obtain the normal `summarizer.review` result first, inspect its destination,
credential reference, limits and history-transfer disclosure, then save this
JSON using that result's **exact** `options` and `digest` values:

```json
{
  "version": 1,
  "options": {
    "profile": "summary",
    "max_output_tokens": 1024,
    "timeout_seconds": 30
  },
  "digest": "<the full SHA-256 digest from your reviewed result>"
}
```

The angle-bracket text above is explanatory; an actual file requires the real
64-character lowercase digest. Do not put credential values in this file.
The file must be a regular file of at most 16 KiB; symlinks, unknown fields and
trailing JSON are refused. Print mode does not accept this option.

On each startup, Hand recomputes the review against the current profile and
rejects a mismatch before constructing the summariser provider. If the profile
changes, start without this option, review the new configuration and explicitly
replace the saved selection. A digest records configuration consistency; it is
not a signature or a substitute for protecting local configuration files.

Follow-main selections use `"follow_main": true` with an empty or omitted
`profile`. Their saved digest binds the main route at startup, so changing the
startup main model requires a fresh review. Subsequent main-model switches in
that process follow the reviewed mode. Interactive/RPC selection does not
automatically overwrite this file; persistence is explicit.

Development validation covers strict loading, application-level restart and
changed-destination refusal. The built CLI RPC journey also verifies exact
selection restoration, follow-main mode and refusal after a destination change;
see `summary-startup-integration.json` and `summary-rpc.json`. Final candidate
and terminal qualification remain pending.
