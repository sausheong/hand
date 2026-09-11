# User-facing messages

Use short, plain sentences for progress and results. Keep internal status and
reason codes in structured events and saved records; translate them for the
terminal. Retain useful underlying errors, paths, limits and next steps.

- Ordinary success: `Completed`.
- Success with required checks: `Completed — checks passed`. This refers only
  to the configured checks, not a guarantee of correctness.
- Cancellation: `Cancelled`.
- Failed checks: `Checks failed: <details>`.
- Provider or execution errors: `Could not complete the request: <details>`.
- Exhausted limits: identify the limit, such as `Session token limit reached`.
- Context summarisation: `Summarising conversation...`; if unnecessary,
  `No summary needed — the conversation is still short`.
- Missing context usage: show `ctx usage unknown / 1M limit` when the limit
  is known. Do not report zero usage without evidence.

Changing presentation must not change exit codes, structured statuses, saved
verification evidence, permission scope, or model/provider error details.
