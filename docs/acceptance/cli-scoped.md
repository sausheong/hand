# CLI scoped-policy cutover

CLI, TUI and RPC startup now use scoped permission authority outside the workspace. The private authority path is under ~/.hand/authority, partitioned by canonical workspace and effective configuration fingerprint. A changed configuration starts a distinct authority context. A workspace encompassing that authority location is rejected rather than silently using project-writable authority.

Use `hand --permissions` to inspect active grants and a legacy migration proposal without provider credentials or model calls. Add `--ack-legacy-permissions FINGERPRINT` only after reviewing the proposal's broad per-tool meaning. Add `--revoke-permission ID` to persist a revocation. These mutation options require --permissions. Legacy settings alone no longer authorise operations; settings reads are bounded, rooted, no-follow and nonblocking, and require regular files.

One-shot --yes remains an explicit broad approval for that invocation and does not create persistent grants. Other one-shot mutations need scoped authority; interactive/RPC clients use the shared approval broker. SDK scoped mode remains explicitly selectable through AuthorityDirectory during migration.

CLI race tests, vet and build pass, including credential-free inspection, explicit legacy import and changed-configuration reapproval. The Python RPC lifecycle and six completion/cancellation compatibility journeys also pass using local providers. Fixture HOME directories are now separate from workspaces to preserve the external authority boundary. Evidence: `cli-scoped-integration.json`.

Full TUI migration UX, isolation, atomic mutation preconditions and final native/released-candidate acceptance remain pending. This cutover does not establish host execution sandboxing.
