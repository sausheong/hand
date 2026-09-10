# Stale file-approval snapshots

Scoped file-write approvals record the target's existence, permissions, size and SHA-256 before asking for a decision. The preview includes that fingerprint alongside exact arguments. After approval, Hand revalidates arguments, resolved path and file state; changed contents, permissions or a newly created target require a fresh review.

Snapshots accept regular files up to 16 MiB, use nonblocking no-follow opens on Linux/macOS, check cancellation while hashing and reject unstable reads. External write previews require read authority for the target. Unsupported safe-open platforms fail explicitly.

Targeted race tests cover stale content, chmod, target creation, symlink changes, FIFO nonblocking rejection and oversized-file rejection. Vet passes. Existing scoped resource-grant tests remain green.

This guards changes while approval is pending; it does not atomically bind a later filesystem mutation to the checked image. The execution boundary, CLI migration/default cutover and full M5 acceptance remain pending. Evidence: `approval-snapshot-integration.json`.
