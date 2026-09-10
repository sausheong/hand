# MCP ownership audit

The audit found that Harness RunTurn did not acquire the exclusion lock used by streaming Run and dynamic MCP attachment. RunTurn now holds that lock throughout execution, including event callbacks, and rejects overlapping operations before session mutation. This closes the dynamic-attachment race for both runtime entry points.

Hand's optional manager now reports an untransferred connection as unavailable only after its client closes. Retry therefore cannot overlap the previous client's cleanup. A real stdio regression closes the manager while a connected client waits for an idle application boundary, then checks that the child has joined and no tools were published.

Full Harness race-suite evidence is in `harness-mcp-audit.json`. Full Hand race-suite evidence is in `mcp-audit-integration.json`. The first concurrent full-suite run failed two Hand healthy-fixture startup deadlines; those failures remain preserved. Healthy-child initialization now uses Hand's configured 15-second startup deadline. The independent 50 ms hung-server timeout test and its two-second return bound are unchanged. The corrected full Hand run passed; this does not qualify native responsiveness or relax any acceptance-contract threshold.

The aggregate Hand patch excludes the temporary go.mod replacement. The development Harness candidate is recorded in the manifests. Released dependency integration, native platforms, full journeys, coverage gates and final acceptance remain pending; M3.4 is still in progress.
