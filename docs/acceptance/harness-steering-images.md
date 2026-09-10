# Image-capable Harness steering

SteeringMessage now carries images, with 16-image/32-MiB aggregate byte limits and an 8-MiB encoded text limit. Session persistence externalises image bytes into the attachment store. Duplicate delivery compares hydrated message data semantically through MatchesSteering, so inline source bytes match persisted blob references. A changed image under the same ID is rejected.

Steering tests passed 20 race-enabled repetitions (100 events), including image persistence/reopen after failed acknowledgement, idempotent retry, changed-image rejection and validation before tool-history mutation. Full Harness/Hand race suites passed 915/637 tests/subtests with no failures/skips; both vet checks passed. The final encoded-text guard was included in the full suite after focused repetitions.

See [patch](harness-steering-images.patch) and [hashed evidence](harness-steering-images.json). The API contract is in runtime/STEERING.md in the patch.

Hand steering still needs attachment snapshot capture and reconciliation integration. Adversarial hydration-bound review, process-kill delivery and full platform/released-candidate qualification remain pending. This is an unpublished Harness candidate; primary Hand remains on v0.3.9.
