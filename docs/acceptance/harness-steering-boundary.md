# Harness steering boundary

Added context-scoped steering to Run/RunSync. Enabling a source disables streaming tool kickoff and makes execution serial, allowing corrections between joined tools. Pending proposed calls receive explicit skipped results before the correction is appended and flushed. A stable ID makes correction delivery idempotent; acknowledgement follows persistence, and errors stop execution.

Tests verify one tool executes while a remaining tool is skipped, call/result pairs and correction survive reopen, identical redelivery does not append twice, persistence failure does not acknowledge, and source/validation errors do not write. Twenty race-enabled repetitions passed (60 events). Full Harness/Hand race suites passed 913/600 tests/subtests without failures/skips; both vet checks passed.

See [patch](harness-steering-boundary.patch) and [hashed evidence](harness-steering-boundary.json). The API contract is in runtime/STEERING.md in the patch.

This is an unpublished Harness seam. Hand still needs queue claiming, acknowledgement handling and terminal steering controls; RunTurn integration and crash-boundary qualification also remain pending. It does not interrupt a tool already executing or awaiting approval. Full M3.2 and final acceptance remain open.
