# Nested guidance before writes and edits

In the development implementation, `write_file` and `edit_file` load applicable
nested HAND.md/AGENTS.md guidance before changing a file. Guidance is ordered
from broader to nearer directories; HAND.md wins over AGENTS.md at the same
level. Root and ancestor guidance already loaded into the prompt is not repeated.
New files inherit guidance from existing parents even when further directories
do not yet exist. Unrelated siblings and outside-workspace instructions are not
loaded.

When applicable guidance or discovery diagnostics exist, a call without the
current `instruction_digest` returns the report, source provenance and digest
without making the mutation. The agent reads that report, adjusts the proposed
change to comply with applicable instructions, and repeats the tool call with
the returned digest. The digest binds the target path and current report. A
changed report invalidates an earlier digest. Calls with no nested report retain
the existing one-call behaviour.

This is an instruction-discovery mechanism, not a new user approval or an
execution capability. Existing permission hooks, scoped approval, isolation and
path rules still apply. A matching digest does not prove semantic compliance,
and does not establish atomic protection against guidance or ancestor changes
concurrent with the final filesystem operation.

Both the host registry and exact-path subprocess worker use the wrappers. The
underlying file tool still validates the edit/write and performs the mutation.
Discovery keeps the existing 32 KiB per-file, 128 KiB total and depth bounds;
unreadable, excluded or colliding sources remain visible as diagnostics.

The shared Harness path validator now resolves the nearest existing ancestor
when creating missing descendants. It rejects outside ancestors and dangling
symlinks instead of accepting an unresolved fallback path. Concurrent ancestor
replacement still requires separate descriptor-relative qualification.

Development regressions cover first-call non-mutation, stale digests, successful
writes/edits, worker parity, new directories, outside paths and cancellation.
See [Hand evidence](mutation-guidance-integration.json) and
[Harness path checks](harness-creation-path.json). Built worker/CLI journeys,
search-result guidance and broad post-change qualification remain pending.

Search follow-up: `search-guidance-integration.patch` now includes bounded guidance for matched directories, with explicit omission notices and `instruction_guidance_complete` metadata. This implements the search-result gap noted above; built-worker and final qualification remain pending. See `search-guidance-integration.json` for focused evidence.
