# Independent MCP construction

Harness construction now uses at most four connection workers. Each server retains its deadline, and results retain configuration order regardless of connection completion order. A required failure cancels sibling construction; every worker is joined before the builder returns and established clients are closed on failure. Optional failures remain visible without publishing tools. The builder publishes the successful catalogue only after all construction workers finish.

The concurrency regression drives the real MCP connection path through an injected HTTP transport that holds four requests open. It verifies all four start independently, the concurrency cap holds across eight configured servers, and status order remains stable. Existing real stdio tests cover connected-client lifetime, partial cleanup, required failures and optional degradation.

The first full Harness race run found an existing diagnostic contract: invalid configuration errors must contain lowercase `mcp server`. The validation message was corrected without changing that test. Both failed and corrected raw runs are retained in `harness-mcp-concurrent.json`. Affected Hand agentio and CLI suites passed against the concurrency commit before the diagnostic-only correction; final exact-candidate Hand qualification remains pending.

The UI still waits for construction before opening its main editor, and retry controls remain unfinished. This work does not complete M3.4 or native/release acceptance. The aggregate Hand patch excludes the local go.mod replacement.
