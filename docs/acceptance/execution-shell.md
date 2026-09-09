# Shell tool execution boundary

The development Harness `BashTool.Backend` accepts an explicit `execution.Backend`. When configured, the tool sends `/bin/bash -c` and the exact approved source to that backend. It does not rewrite paths based on the host filesystem. Existing policy denial runs before backend admission; a backend error is surfaced without host fallback. A container image must contain `/bin/bash`; an absent executable fails visibly.

Backend execution accepts a caller-owned artifact store. The shell tool retains bounded inline output, complete capture artifacts subject to existing quota limits, byte counts, per-stream truncation, exit/cancellation status and the backend's stated boundary. Artifact errors are visible in output as well as metadata.

Regression tests verify exact source despite a host Unicode-whitespace path match, policy rejection before backend execution and absence of host side effects after backend failure. A real cached Debian container test verifies root-write denial, explicit workspace writes, a 100,000-byte stdout capture and retrieval through the artifact integrity reader. The affected shell and execution race suites passed with native fixtures configured; vet passed. Raw evidence is in `harness-execution-shell.json`.

This patch follows `harness-execution.patch` and remains an unpublished Harness change. Hand backend selection, other built-in tools, background operations, MCP/extensions, Linux-host testing and final acceptance remain pending. The legacy unconfigured BashTool path retains its existing behaviour for compatibility; Hand will select an explicit backend as the integration proceeds.
