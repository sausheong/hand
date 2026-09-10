# Durable RPC request ledger

The staged ledger synchronises an intent before granting execution, binds it to a run ID, then stores a completed result. It locks the ledger exclusively on Linux/macOS, requires private storage, and bounds retention to 1,024 requests, 64 KiB per record and 64 MiB per log. File and directory synchronisation protect recorded admission. Unsupported platforms fail explicitly.

A matching duplicate returns the existing record without an execution grant. Reuse with a different method, session or parameter encoding is rejected. Parameter whitespace is compacted; object key order remains significant, so clients should replay the same request representation. IDs are scoped to the ledger, and results are copied on return.

After reopening, pending and accepted records become uncertain; they cannot silently start another run. Completed records return their stored result. A truncated or invalid log fails closed for reconciliation. Capacity exhaustion is explicit; no automatic eviction may remove a duplicate-prevention record. Administrative retention and uncertainty reconciliation are not yet implemented.

Five permanent race tests passed: duplicate/restart behaviour, completion replay/copying, writer exclusion/truncation, pending intent recovery and concurrent admission. Vet passed. Earlier test failures exposed omitted Go parameters marshalling as null; Begin now normalises omitted parameters to an empty object before validation. All raw runs are retained in `rpc-ledger-integration.json`.

The ledger is not yet wired to RPC dispatch. These ordinary close/reopen tests do not substitute for process-kill fault injection, final filesystem durability qualification or a real non-Go client journey. M4.1 remains in progress.
