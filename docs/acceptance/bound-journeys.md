# Built-binary permission and interface journeys

The full development race suite and fresh built-binary checks are recorded in `bound-journeys.json`. The binary is built from the staged Hand checkout using the unpublished Harness candidate in `permission-binding-integration.json`.

The permanent runner `scripts/check_permission_profiles.py` starts a real RPC subprocess and local HTTP provider. It selects profile `first`, approves a file write persistently, switches to `second`, verifies an empty grant list and an approval prompt, denies the write and checks unchanged content, then returns to `first` and confirms the original grant permits the next write without prompting. It also verifies the six provider requests use the expected model sequence. The runner archives protocol requests/responses, provider requests, binary and runner hashes, and exit/cleanup outcome. Its first execution failed on an incorrect assumption about JSON field casing; the corrected execution is recorded separately.

The existing Python lifecycle runner passed approval-before-write, steering, queue controls, duplicate-request handling, version rejection, cancellation and reconnect recovery. The external SDK consumer passed completion and cancellation through CLI JSONL, subprocess SDK and embedded SDK. All providers were local fixtures; no live evaluation was performed.

Run against a built development binary with a new output directory:

```sh
python3 scripts/check_permission_profiles.py --hand-binary /absolute/path/to/hand --out /absolute/path/to/new-evidence-directory
```

This evidence supports development integration only. It does not satisfy exact clean release, published Harness, full coverage, native platform, performance or live comparison gates.
