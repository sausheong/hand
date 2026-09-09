# Scoped permission model development core

The new scoped policy model binds a grant to an operation, exact or filesystem-tree resource, canonical workspace, configuration digest, lifetime and provenance. Operations distinguish file reads/writes, skill mutations, structured executable/argument vectors, raw shell commands, network origins and MCP server/tool pairs. Invocation and session lifetimes require matching identities; persistent grants cannot ambiguously carry narrower lifetime fields.

Inspection returns copied grant values and revocation takes effect under the same lock as lookup. Project-policy provenance is rejected. Configuration changes require a different authority decision. File scopes resolve symlinks, including parents of new files, reject unresolved links and compare path components rather than string prefixes. Commands and shell scripts require exact matches. Approval of a host command does not sandbox its children or the code it executes.

This is an application policy core, not yet connected to production approval hooks. It does not load authority from project files. External authority persistence, acknowledged legacy migration, approval inspection/revocation UI and stale-preview revalidation at the execution boundary remain required. Path matching alone cannot prevent filesystem changes between approval and execution; rooted tool access remains necessary.

The permission race suite and vet pass. New tests cover lifetime/configuration mismatch, copied inspection, immediate revocation, symlink/prefix escapes, exact command/network/MCP identities and rejection of project provenance. No M5 acceptance requirement is closed. Evidence: `scoped-policy-integration.json`.
