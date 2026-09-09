# Harness writer lease follow-on — local evidence

Commit `dfae3db0f2f016089e4be29f14d97bee40f24e4b` follows atomic-rewrite candidate `c6c155d314f176527d0dd3e33253d592bf82ef61` in `/private/tmp/hand-harness-persistence-20260908`. It is local and unpublished; Hand still uses Harness v0.3.9.

`Store.LoadExclusive` acquires a nonblocking kernel writer lock before loading and retains it until `Session.Close` or process death. Close joins synchronous writes and is idempotent; subsequent persistent writes through that session fail. A lease is bound to its original store, agent and key. Advisory PID contents never authorise takeover. Lock files remain at stable paths under each agent's private .leases directory; they are not deleted on release.

Updated append, rewrite, sync, create, delete and rename operations respect an exclusive lease. Legacy callers without a lifetime lease acquire temporary operation leases on supported systems, preventing them from bypassing an exclusive owner. Two legacy callers still do not form a safe DAG-writing protocol; use LoadExclusive. This coordinates cooperating updated clients, not arbitrary filesystem writers or older library versions.

Linux/macOS use flock with close-on-exec and no-follow lock-file opening. Other platforms reject LoadExclusive explicitly while retaining legacy operation behaviour. No active lock is forcibly stolen based on age or PID. The subprocess regression acquires a lease in a child, verifies the parent cannot acquire it, kills the child, joins it and successfully reopens/writes the session.

The full session suite passed 20 race-enabled repetitions, 780 tests/subtests, including the subprocess case. Fresh full Harness race/coverage validation passed 786 tests/subtests, zero failures/skips, on the local macOS host. Session/runtime vet passed. Session test binaries also compiled for Linux arm64/amd64 and macOS amd64; they were not executed on those targets. Raw evidence, hashes and compilation artifacts are recorded in harness-lease.json; the incremental patch is harness-lease.patch.

Truncated-record recovery under the lease, durable selected branches, catalogue/migration, all deferred/background persistence paths and Hand integration remain unfinished. Native Linux/other-target execution, hosted release checks and publication authorisation remain absent. This is not full M3.1 acceptance.
