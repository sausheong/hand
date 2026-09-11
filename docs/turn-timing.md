# Turn timings

After each turn, Hand shows the total time, model request count, time to the first text, and time spent checking the result. Use `/timing` for the latest turn's detailed report, including while it is running. Plain command-line runs print the summary to stderr.

The report includes:

- Time between starting the turn and the first model request.
- Each provider call's start, duration, streaming or fallback mode, and outcome.
- Connection ready, whether the connection was reused, request sent, first response byte, first provider event, first text and first tool call.
- Waiting from request sent to the first response byte, and time from first text to the end of the stream.
- Reported input, output and cached input tokens. Missing usage stays unknown.
- Completion checks, cleanup, observed permission waits, and completed tool spans.

For example, a quick connection followed by a long wait before the first response suggests the delay is after connection setup. If headers arrive quickly but text arrives much later, the delay is in the response stream before text. A long interval after the first text shows time receiving and delivering the answer. A long interval before the first model request points to work in Hand's turn preparation. These are clues, not measurements of the gateway's internal queue or the model's internal processing.

Reports are saved as versioned `hand.timing` annotations in the existing session JSONL, associated with the run ID. Session export retains them. `/timing` shows this process's latest turn; it does not load historical reports after restart. A failed telemetry save gives a warning without changing the task's outcome.

## Measurement boundaries

Durations use the monotonic clock and are stored in milliseconds. HTTP milestones are offsets from the provider call's start. Runtime budget admission happens before this timer. The connection-ready milestone combines connection acquisition, DNS, TCP and TLS; it does not separate these costs. HTTP tracing depends on the provider transport; absent callbacks are displayed as “not observed”. For a call with internal HTTP retries, the report records the first observed HTTP milestones, not a separate breakdown of every transport attempt. The existing usage journal retains billing request IDs and categories separately.

The timer begins when the application service executes the turn. It excludes process startup, earlier configuration/skill discovery, attachment preparation before the service starts, and saving/displaying the timing report itself. Cleanup measures operation close and final workspace capture after model execution; remaining bookkeeping is included in the total. Checks include any operation completion gate. Tool spans run from the runtime's tool-call event to its result, so they include permission waiting and event delivery. They are not pure process CPU time. A tool without a matching result has no completed span. Request durations include local event delivery and backpressure. Overlapping requests, approvals and tool spans must not be added together as exclusive phases.

Detailed records are bounded to 128 model requests and 128 tool spans per turn. Additional requests are counted as omitted. Telemetry stores timing, counts, tool names and usage metadata; it does not store prompts, answers, tool arguments, credentials or endpoint URLs. Existing session content storage is unchanged.

## Verification

Tests use fake providers and a local HTTP streaming server, without paid model calls. They check delayed first text versus later streaming, HTTP connection reuse, usage observation and budget settlement exactly once, preservation of optional provider capabilities, cancellation, incomplete and failed streams, concurrent snapshots, bounded records, session annotations, and availability before the terminal event. Run:

```sh
go test -race ./internal/app -run 'TestTiming|TestServiceTiming'
```
