# Scoped approval hook integration

ToolAccess derives file read/write paths, exact shell scripts, network origins and MCP server/tool pairs from tool arguments. Unsupported operations use an exact tool/input identity. Scoped approvals show the operation, resource and exact arguments; tool input is bounded to 64 KiB. After a decision, arguments and resolved resources are revalidated. Persistent decisions enter the external authority store as exact-resource grants, not broad per-tool permissions.

The shared application broker now supports this hook, and EmbeddedOptions.AuthorityDirectory opts into it with private external storage and a fingerprint of the embedded model/endpoint/credential-reference/run-limit configuration. Workspace reads are routine; external reads, network and other capabilities require authority. Empty AuthorityDirectory retains the existing approval path during migration.

Targeted race tests verify that an approved file scope suppresses repeat prompts only for that path, broader paths prompt again, symlink changes during approval are rejected, and external/unknown operations require decisions. A scoped SDK test verifies session switching/reopening and authority cleanup. Vet passes; the initial missing-constant build failure is retained.

CLI acknowledgement UI/default cutover is still pending. Resolved-path revalidation does not yet verify stale file-content previews, close every execution-time filesystem race, or enforce redirect destinations. Execution isolation and the rest of M5 acceptance remain required. Evidence: `scoped-hook-integration.json`.
