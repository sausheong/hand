# Explicit legacy migration in the TUI

`/permissions legacy` displays the immutable startup migration proposal: broad per-tool authority, the exact tool names, canonical workspace, configuration digest and acknowledgement fingerprint. Reading this proposal grants nothing. `/permissions acknowledge <fingerprint>` imports only the matching reviewed proposal through the external authority journal. Imported grants retain `legacy_acknowledged` provenance and can be inspected or revoked through existing permission commands. Repeated acknowledgement resumes the same proposal without duplicate grants.

The proposal is captured at startup; changes to project settings require restarting to prepare a new proposal. Project settings cannot import their own grants. This implementation does not execute acknowledgement on behalf of the real user.

The regression `TestPermissionLegacyReviewRequiresExactAcknowledgement` verifies displayed scope, zero authority after review or a wrong fingerprint, exact legacy operation/provenance after acknowledgement and idempotence. The combined TUI/CLI race suite and vet passed; raw evidence is recorded in `tui-legacy-integration.json`.

The aggregate patch builds on TUI permission inspection and requires the development Harness candidate recorded in its manifest. Released dependency integration, dynamic profile authority binding, atomic mutation preconditions, native isolation and final acceptance remain pending.
