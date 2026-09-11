# Empty answers and output limits

Hand must not report a completed answer when the model returns no visible text
and no tool calls. A generic empty response gets at most one retry in the agent
loop, provided no text or tool activity has begun. The retry uses the same model,
request and output allowance and passes through the existing budget admission
and cancellation checks. A second empty response fails clearly. The single-turn
API reports the failure directly so its caller owns retry decisions.

An explicit `length` or `max_tokens` stop reason means the answer was cut short.
Hand reports the output limit instead of declaring completion, including when
partial answer text was received. It does not automatically increase the limit
or replay tool actions. OpenAI-compatible providers preserve the final stop
reason and trailing usage; incomplete tool arguments are not executed.
Truncated summaries are rejected, retaining the original conversation history.

## Setting the output allowance

Input context capacity and output allowance are separate settings. For example,
with a configured context limit larger than 8,192 tokens:

```sh
hand --max-output 8192
```

This applies only to that invocation. For persistence, add `"max_output": 8192`
to the existing top-level config for a legacy model configuration, or inside the
selected named profile. Named profiles keep their own limits. Explicit limits
must fit below the resolved context capacity and must be supported by the server.
Existing defaults and explicitly configured limits are preserved.

`/timing` now records each request's output allowance and provider stop reason.
A request cut short by the output cap has status `output_limit`. Provider request
completion remains distinct from successful completion of the user's work.

## Verification

- Harness runtime: empty responses, bounded retry, successful recovery,
  reasoning-only/whitespace output, partial and empty output-limit termination.
- Provider: trailing usage and stop reason survive streaming; incomplete tool
  calls are not emitted for execution.
- Compaction: truncated summaries cannot replace conversation history.
- Hand integration: the reported 2,048-token failure becomes a failed outcome
  and an output-limit timing record through the real HTTP adapter and runtime.
- Configuration and CLI: output allowances reach provider requests and named
  profiles retain their own explicit values.

Hand v0.3.4 pins Harness v0.4.2, which includes these fixes. Tests use local
mock providers; no live gateway verification has been performed.
