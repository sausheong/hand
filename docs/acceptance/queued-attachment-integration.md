# Queued follow-up attachment resolution

NewHarness now configures a workspace input resolver for follow-ups. StartFollowup snapshots the oldest entry, resolves files/images outside the owner lock, then rechecks queue identity, text, session/order, cancellation and run ownership before admission. Current profile image capabilities apply. Failed resolution or a concurrent edit/removal leaves the queue intact and allocates no run.

Tests cover image bytes and current file snapshots reaching the backend, missing-file/profile rejection without queue loss, and a held resolver with a concurrent edit that cannot submit stale input. They passed 20 race-enabled repetitions. Full Hand validation passed 637 tests/subtests with zero failures/skips; vet passed.

See [usage](queued-input-usage.md), [aggregate patch](queued-attachment-integration.patch), and [hashed evidence](queued-attachment-integration.json).

Steering attachments, external policy, durable queue restart and native responsiveness/final acceptance remain pending. Custom application Service constructors must supply ResolveInput for attachment support; NewHarness wires it automatically. Integration remains staged against unpublished Harness; primary Hand uses v0.3.9.
