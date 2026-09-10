# External SDK compatibility runner

`scripts/check_sdk_compatibility.py` builds a real external Go module from `examples/compatibility/main.go`, using only public SDK/protocol imports. The executable compares CLI JSONL, subprocess SDK and embedded SDK against one local HTTP provider fixture. Each interface must deliver the expected text, one completed terminal, and matching durable SDK terminal state; exactly three provider calls are required.

Development invocation requires `--checkout`, `--harness-checkout`, `--fixture`, `--hand-binary` and a new `--out` directory. Released invocation substitutes `--version vX.Y.Z` for checkout arguments. It refuses replacement dependencies, a binary version without the expected release and concrete commit, and a binary Harness dependency that differs from the external module. Released mode has not yet run.

The output retains command arguments, exit codes, stdout/stderr hashes, fixture/runner/binary hashes, dependency inventory and the external module source. Local fixture access is required; no live model calls are made. Results are scoped to successful completion equivalence. Cancellation, approvals and the complete interface acceptance contract still need broader journeys.

The final development run passed with identical observations for all three interfaces. An initial failed module-inventory run is retained; it attempted unrelated dependency lookup with GOPROXY disabled. The runner now inventories the two required dependencies directly. Evidence and the aggregate implementation patch are indexed in `sdk-compat-integration.json`.
