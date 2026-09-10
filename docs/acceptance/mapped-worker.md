# Workspace path mapping for tool workers

The parent proxy maps file and search paths from the trusted host workspace to `/workspace`. It canonicalises the root and target, checks component containment and rejects outside paths, traversal and symlink escapes. A missing search path maps to the workspace root. File contents and other argument values are preserved; arbitrary shell strings and URLs are not rewritten. Additional external mounts require an explicit mapping mechanism and are not silently substituted.

Harness file tools now offer `ExactPath`. The worker enables it for read/write/edit, disabling home expansion and Unicode-whitespace path recovery. The ordinary default tool behaviour remains compatible. This prevents a differently spelled path from silently resolving to another file during worker dispatch.

Tests cover relative and absolute workspace paths, content preservation, prefix-confusion/traversal/symlink rejection, default search paths and rejection of a Unicode-whitespace substitute. The native proxy roundtrip and large-response test passed with the newly built worker, and existing Harness file-tool race tests passed. Vet passed for affected packages. Evidence, hashes and patches are recorded in `mapped-worker-integration.json`.

These checks do not close the remaining interval between approval and filesystem mutation. That requires execution preconditions and further race tests. Startup isolation selection, complete routing, external-mount support, native Linux-host qualification and final acceptance remain pending.
