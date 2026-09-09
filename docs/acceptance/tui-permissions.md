# TUI scoped permissions

The staged implementation provides `/permissions [offset]` to inspect persistent scoped grants and `/permissions revoke <ID>` to durably revoke one. Help and completion include the command. Inspection shows operation, scope, resource, lifetime and provenance in pages of 16. Long resource and identity labels are truncated; use `hand --permissions` for full values.

Authority reads and writes run in an owned worker because either can wait on a journal fsync. The TUI remains responsive and supports permission controls during a foreground run. Shutdown joins the worker before the authority can close; stale completion messages cannot resurrect a completed operation. Revocation affects future admission, not work already executing.

`TestPermissionCommandsPersistRevocationAndPage` exercises real authority storage, paging, foreground-state preservation, revocation with shutdown before message consumption, and reopening. The full TUI race suite and vet passed. An initial test used a noncanonical temporary workspace and correctly failed the authority context check; its raw output is retained.

This aggregate patch builds on the CLI scoped integration and requires the unpublished Harness development candidate recorded in the manifest. It is not applied to primary Hand and does not establish final acceptance. TUI legacy acknowledgement, atomic file mutation preconditions, native isolation and final platform/release qualification remain open.
