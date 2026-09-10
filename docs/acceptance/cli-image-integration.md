# One-shot image-input parity

One-shot execution now parses image paths through the same strict parser as normal TUI submission and passes image bytes into the application service. Missing attachments return an explicit attachment-input failure. Text-only profile admission rejects images before provider calls or run-journal writes. Cancellation retains the cancelled outcome.

Tests exercise a quoted path with spaces through the real runtime/provider request path, plus missing-image and text-only-profile rejection. They passed 20 race-enabled repetitions (80 tests/subtests). Full Hand validation passed 627 tests/subtests without failures/skips; vet passed. These are one-shot function integration tests, not standalone binary image qualification.

See [usage](image-input-usage.md), [aggregate patch](cli-image-integration.patch), and [hashed evidence](cli-image-integration.json).

External attachment policy, queued images, content validation, file completion and final native/released-candidate qualification remain pending. Integration remains staged against unpublished Harness; primary Hand uses v0.3.9.
