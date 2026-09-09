# Packages and extensions

A package is a pinned collection of files. Installing it does not execute its
extensions or load its skills. Extension activation is a separate review bound
to selected bytes, capabilities, workspace and execution boundary. This guide
covers the development implementation; release compatibility remains pending.

## Inspect and install a local package

Use a private package store outside the workspace. The default is
`~/.hand/packages`. Start by inspecting an explicit package directory:

```sh
hand packages inspect --source /absolute/package
```

Review its manifest, file inventory, compatibility and `package_digest`. Use
that exact digest as `PACKAGE_DIGEST` when preparing installation:

```sh
hand packages review-install --source /absolute/package \
  --store /absolute/private/package-store --pin PACKAGE_DIGEST \
  --out /absolute/new-install-review.json
```

Read the resulting review and proposed changes. If accepted, copy its
`approval_digest` into:

```sh
hand packages apply --review /absolute/new-install-review.json \
  --approve APPROVAL_DIGEST
```

Review files must use new paths. An approval belongs to the reviewed bytes and
store state; if either changes, prepare and inspect another review. Do not
construct approval digests yourself to skip review.

`review-archive` additionally requires `--archive-sha256` for the source archive's
bytes. `review-git` requires a full `--commit` for an explicit Git repository.
Both still require the package content pin. These are explicit source operations,
not a package registry search or permission to track an unpinned branch.

## Update, remove or roll back

`hand packages list --store STORE` shows the lockfile and recovery report.
`hand packages show --store STORE --name NAME` inspects a selected installation.
To update, inspect the new source and prepare another `review-install` with its
new content pin; review the change before applying it.

For removal, prepare `review-remove --store STORE --name NAME --out NEW_REVIEW`.
For rollback to a retained revision, use
`review-rollback --store STORE --name NAME --pin RETAINED_DIGEST --out NEW_REVIEW`.
Both changes use the same `apply --review ... --approve ...` step. Retain the
relevant package pins and reviews. Do not edit the store's lockfile or cached
objects by hand. A store or source integrity failure must be resolved before
activation, rather than selecting altered bytes under an old pin.

An already selected skill or launch snapshot is not silently redirected by an
update. Review and select the new revision explicitly. For installed skill bodies,
relative resources and package prompts, see [package skills](package-skills.md).

## Review extension activation

For a native installed extension without an external interpreter:

```sh
hand packages review-extensions --store /absolute/private/package-store \
  --names task-note --workspace /absolute/project \
  --snapshots /absolute/private/extension-snapshots \
  --out /absolute/new-extension-review.json
```

Review the launch configuration and its `approval_digest`, then start Hand from
that workspace with the reviewed file and digest:

```sh
hand --extension-config /absolute/new-extension-review.json \
  --approve-extension-config APPROVAL_DIGEST
```

This host-mode approval authorises unrestricted host execution of the reviewed
extension. Declared host API capabilities do not sandbox its executable. For
container execution, use the explicit `--container` configuration supported by
`review-runtimes` and `review-extensions`; review the image, runtime mapping and
boundary. Do not fall back to host execution when container admission fails.

Interpreted packages require a separate `review-runtimes` step. Inspect the
runtime paths, digests and constraints, approve the runtime review explicitly,
and copy the matching value from the command output's `review_digests` into that
entry's `approved_digest` field in the review document. Pass the document through
`--runtime-approvals` during launch review. Launch review may then run the approved
interpreter version probe. A
runtime review is distinct from installing the package or approving the final
extension configuration.

Once activated, invoke a declared command with
`/extension EXTENSION-NAME COMMAND [arguments]`. The equivalent RPC integration
must handle structured questions and explicit user decisions; absence of a
terminal must not silently grant approval. Keep stdout reserved for protocol
frames and diagnostics on stderr when writing an extension peer.

## Build from the examples

The [example guide](../examples/extensions/README.md) explains Go/Python task-note,
policy-veto, verification-hook and tool-viewer peers, including capability sets
and their limits. Build a native package and inspect its generated pin with:

```sh
python3 scripts/package_extension_example.py --source-root /absolute/hand-checkout \
  --hand-binary /absolute/path/to/hand --example task-note \
  --out /absolute/new-example-build
```

The script compiles and inspects the package without running the extension.
Its `run.json` records the package directory, pin and build evidence. Use those
values in the review/install workflow above. The package's declared minimum Hand
version must be satisfied by the selected binary.

Use MCP when exposing ordinary service tools or data is sufficient. Extensions
are for Hand-specific commands, structured interaction, state, context transforms,
policy vetoes and presentation. Neither mechanism bypasses the effective host
policy; their operating-system reach also depends on the chosen execution boundary.
