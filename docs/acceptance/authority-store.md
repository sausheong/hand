# External persistent approval authority

The Authority API stores persistent scoped grants and revocations in an 8 MiB bounded JSONL journal, with records below 64 KiB. It requires a private directory outside the canonical workspace and a regular private file, opened without following a final symlink and held under an exclusive writer lock. Unsupported platforms fail explicitly.

Grant and revoke changes are synced before becoming active. A persistence failure disables authority rather than leaving an unpersisted grant active. Reopening validates record shape, provenance, workspace/configuration identity and canonical saved resources. A saved path that now resolves through a different symlink requires reapproval. Truncated journals fail closed. Only persistent grants are written; invocation/session grants belong to the in-memory policy lifetime.

Tests cover grant/revocation reopening, exclusive ownership, refusal of project-local storage, truncation, changed saved-resource symlinks and failed writes. The permission race suite and vet pass. This storage API is not yet connected to production approval hooks, legacy migration or inspection/revocation interfaces.

Placement outside the workspace prevents project files from acting as the authority source; it does not sandbox arbitrary host code running as the same OS user. Execution-backend isolation remains a separate required boundary. Full crash/native/release qualification remains pending. Evidence: `authority-store-integration.json`.
