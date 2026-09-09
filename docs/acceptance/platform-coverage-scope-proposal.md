# Proposed coverage scope clarification

**Status: proposal only; current acceptance rules remain unchanged.**

The advertised targets are darwin/arm64, darwin/amd64, linux/arm64 and linux/amd64. Fresh `go list -json` for each target proves that the following five files are excluded by their build constraints on every supported target:

- `cmd/hand/legacy_open_other.go` — `//go:build !darwin && !linux`.
- `internal/agentio/approval_snapshot_other.go` — `//go:build !darwin && !linux`.
- `internal/checkpoints/rename_other.go` — `//go:build !linux && !darwin`.
- `internal/permissions/authority_other.go` — `//go:build !darwin && !linux`.
- `internal/rpc/ledger_other.go` — `//go:build !darwin && !linux`.

They return explicit unsupported-platform errors. They are ordinary source, so the current definition-of-done section 2 and coverage classifier do not permit excluding them as tests/generated code. Their missing profiles cannot honestly be called measured.

Proposed amendment: Exclude from statement-coverage denominator only enumerated, fingerprinted source files proved by Go build selection to be absent from all four supported Linux/macOS targets. Keep them explicitly listed as non-applicable; do not exclude supported platform variants or failed tests.

This does not waive Linux or macOS runtime testing, architecture smoke tests, failed tests or scenario coverage. Linux rename code is still required and has separate native-container evidence. A scope entry must be invalidated if its source hash or supported build selection changes. All other gates and the user-approved 80% thresholds stay unchanged.

Evidence: [machine-readable proposal](platform-coverage-scope-proposal.json). No acceptance checker or coverage-scope policy was changed.
