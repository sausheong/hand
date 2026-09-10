# Owned asynchronous Markdown resize

Resize histories with at least eight stale assistant blocks or 64 KiB of stale Markdown now use a single model-owned layout worker. Smaller updates remain immediate. While reflow is pending, the current cached presentation remains available and input handling continues.

New widths/styles cancel obsolete work. A completion is joined before it can update matching source blocks, and it must match the current width, style and history generation. Clearing or replacing history invalidates old work. Shutdown cancels and joins the worker, including when its result command was not consumed. Only one worker exists at a time.

The TUI race suite passed 225 test/subtest events; vet passed. Tests cover rapid resize to the latest width, editable input while work is pending, history clearing and shutdown joining. Evidence is in `markdown-layout-integration.json`.

Cancellation is checked between blocks; an individual Markdown renderer call is not interruptible. Initial source rendering remains synchronous. Native worst-case latency, text-delta coalescing and complete M3.3 qualification remain pending; earlier plain-text benchmark figures do not qualify these Markdown workloads.
