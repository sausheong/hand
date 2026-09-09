# Verified captured-output reader

The staged Harness capture store now hashes bytes written to each artifact and records SHA-256 after closing a synced file. A new store-scoped reader verifies the captured bytes before returning them. It accepts only exact output-file names within its configured store, rejects symlinks and nonregular files, checks private permissions and declared size, takes a nonblocking artifact lock and verifies the digest after bounded, cancellation-aware reads.

Missing or expired files, changed files, active captures and legacy records without a digest fail explicitly. A verified captured prefix can still have `Truncated: true`; integrity does not imply the capture contains all process output.

The process/bash race suites and vet passed. See `harness-artifact-reader.json` for the candidate, patch hash and raw evidence. This does not yet persist artifact references in session records or wire the reader into Hand. Full Harness/Hand regression runs, native durability qualification and released integration remain pending.
