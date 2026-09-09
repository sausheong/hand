# Explicit legacy permission migration

PrepareLegacyMigration produces an immutable proposal from old always_allow settings. Its fingerprint binds the canonical workspace, current configuration identity and sorted deduplicated tool list. Reading settings or preparing a proposal creates no authority.

AcknowledgeLegacy requires the fingerprint that was reviewed. Callers must obtain explicit acknowledgement of the broad meaning before invoking it. Imported grants are labelled legacy_acknowledged and legacy.tool: a bash grant remains broad permission for bash, rather than being silently narrowed into a command-prefix rule. Other tool identities are not included. Ordinary user-decision provenance cannot create this legacy broad-grant type.

Imports persist each grant independently; interruption may leave a subset. Repeating the same acknowledged proposal resumes without duplicate grants. A changed tool list or configuration produces a different fingerprint and requires a new decision. Proposal inspection returns copied values.

Permission race tests and vet pass. Tests verify that proposals and mismatched acknowledgements grant nothing, explicit import preserves broad meaning, tool scopes remain separate, imports resume and changed lists invalidate old acknowledgements. Production migration UI and approval-path integration remain outstanding. No acknowledgement has been obtained or fabricated for the user's actual legacy settings. Evidence: `legacy-migration-integration.json`.
