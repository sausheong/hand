# Reproduce Linux RPC qualification

Use an existing immutable Linux image with `/bin/sh` and the architecture selected
below. The runner builds Hand, its command test executable, the RPC test executable
and the task-note extension from
the current checkout, inventories expected tests inside the container, and refuses
missing tests, skips, failures or fewer than 20 recovery repetitions.

```sh
GOCACHE=/private/tmp/hand-review-gocache GOPROXY=off \
python3 scripts/check_rpc_linux.py \
  --docker /usr/local/bin/docker \
  --arch arm64 \
  --image sha256:7b140f374b289a7c2befc338f42ebe6441b7ea838a042bbd5acbfca6ec875818 \
  --out /private/tmp/hand-rpc-linux-new-run
```

Choose a new output directory outside the checkout. Docker context/DOCKER_HOST and
Go cache/module access come from the caller's environment. The container has no
external networking and a read-only root; its bounded temporary filesystem is
executable because reviewed extension snapshots are copied there before launch.
A supplied `HAND_TEST_RPC_NOTE_BINARY` replaces fixture compilation inside the
container, not the extension behaviour under test. An unusable binary fails.

`run.json` records source inventories before/after, binary hashes, commands, test
inventory, raw-log hashes and results. Timeout cleanup removes only the uniquely
named container created by this runner. A changed source tree fails qualification.
The Linux test executable is CGO-disabled and not race-instrumented; native race
checks must be collected separately. This runner covers the RPC package and the
combined slow-consumer recovery regression, and built Hand framing rejection. It
does not cover all Linux acceptance requirements.
Final qualification still requires the clean candidate and released Harness.

The runner also resolves Harness through `go list -m -json`, records its actual
module directory/version metadata, and fingerprints the dependency before and
after execution. This supports both local checkouts and module-cache directories;
local workspace use is recorded explicitly and does not satisfy the released
Harness gate. Go version, target architecture, CGO setting and workspace path are
recorded with `go env`.

The built Hand check rejects unsupported versions, duplicate JSON IDs and prompts
before negotiation, then successfully negotiates on the same connection. An
oversized frame must close the connection with bounded diagnostics, without
admitting a trailing prompt, contacting the fixture provider, or appending an
accepted request to the private ledger. `HAND_TEST_RPC_FRAME_BINARY` selects the
freshly built child binary; the test executable runs inside the same Linux
container. `built_client_framing_passed` and `framing.log` record this result.
