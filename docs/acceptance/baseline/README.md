# Pre-implementation coverage baseline

Measured 8 September 2026 against Hand application source at `e6dd263`, Harness `v0.3.9`. Documentation and acceptance-validator files were untracked additions during measurement; this is not a clean final release candidate or plan-completion evidence.

Command (exit 0; all six packages passed):

```sh
GOCACHE=/private/tmp/hand-review-gocache go test -count=1 -covermode=atomic -coverprofile=/private/tmp/hand-baseline-20260908.cover ./...
```

The Go cache was placed in a writable temporary directory. The resulting unmodified [coverage profile](2026-09-08.cover) is retained here. Recompute the aggregate with `go tool cover -func=docs/acceptance/baseline/2026-09-08.cover`.

| Package | Statement coverage |
|---|---:|
| `cmd/hand` | 14.3% |
| `internal/agentio` | 91.8% |
| `internal/config` | 85.3% |
| `internal/permissions` | 73.1% |
| `internal/sessionio` | 83.3% |
| `internal/tui` | 83.0% |
| **Total (weighted by statements)** | **75.1%** |

The total is below the new 80% acceptance floor. Existing package coverage does not establish coverage of future critical subsystems, changed-code coverage, actual provider task quality or any unimplemented scenario. The temporary acceptance checker has a separate synthetic test suite; passing those tests establishes only validator behaviour.
