# Optional MCP startup

Set `"optional": true` on an MCP server configuration to permit startup when that server cannot connect. Existing entries default to required. Optional server construction has a five-second default connection timeout in Harness; the overall Hand startup deadline still applies. Owner cancellation always aborts construction and closes partial connections. Required-server failures remain fatal.

Harness validates nonempty unique names before connecting, preventing ambiguous namespace collisions between configured servers. Only successfully connected servers publish tools. Immutable status snapshots contain server name, optional flag, state and tool count without raw transport errors or credentials. Interactive Hand displays these statuses after its banner; one-shot mode reports unavailable optional servers on stderr.

Seven targeted Harness construction tests passed, including actual stdio server success beside a timed-out optional server, required failure cleanup and owner cancellation. Full Hand race-suite results and two subsequent configuration/status wiring tests are preserved in `mcp-optional-integration.json`. Harness evidence is in `harness-mcp-optional.json`. Test-event counts are not acceptance-scenario counts.

The aggregate Hand patch excludes the temporary go.mod replacement. It requires unpublished Harness commit `fd9ee98d2bef92fee2658eabc5b4da49038fcc22`. The UI still starts after connection work: UI-first construction, full Harness qualification, native suites and final acceptance remain outstanding.
