# Hand package manifest contract

Development implementation: `internal/packages` in the staged integration patch. This contract is the foundation for M7.2; installation, updates, lockfiles, rollback and CLI commands are not yet delivered by it.

An explicitly selected package directory contains `hand-package.json`. Reading, decoding, hashing or verifying it does not execute code, approve capabilities or activate an extension. Project discovery must not activate it automatically.

The version-1 JSON object declares:

- `schema`: `1`.
- `name`: a lowercase ASCII identifier, at most 64 characters.
- `version`: a semantic version, including optional prerelease/build identifiers.
- `compatibility`: `minimum_hand` semantic version and `extension_protocol: 1`. These declarations must be checked against the selected Hand version by installation/activation; manifest parsing alone does not establish runtime compatibility.
- `files`: the complete regular-file inventory, excluding the manifest itself. Each has a portable relative `path`, `kind`, lowercase `sha256`, exact byte `size` and optional `executable` flag. Roles are `skill`, `prompt`, `extension`, `source` and `asset`. Skill entry files must be named `SKILL.md`; only extension-role files may declare executable permission.
- `runtimes`: optional interpreter requirements, each with an identifier `name`, bare executable `command` and semantic `minimum_version`. These are metadata, not shell commands. Runtime discovery/version checks and approval remain installation/activation responsibilities.
- `extensions`: entrypoint declarations with `name`, inventoried `entrypoint`, optional declared `runtime`, ordered `arguments` and protocol `capabilities`. An entrypoint must either be executable or use a declared runtime.

The manifest is limited to 256 KiB both on input and in its re-encoded form. Duplicate JSON keys, unknown fields, unsupported protocol versions, duplicate names, case-colliding files and file/directory collisions are errors. There may be at most 1,024 files, 16 extensions and 16 runtimes. Files are limited to 128 MiB each and 512 MiB combined. Paths have at most 32 components and 1,024 ASCII bytes; absolute paths, traversal, backslashes, `.git`, reserved manifest-name variants and control characters are rejected. Extension argument arrays have at most 128 entries and 32 KiB total, with valid UTF-8 and no NUL.

`Manifest.Digest` hashes a canonical manifest with file/runtime/extension inventories and capability sets sorted. It retains command argument order and covers content hashes, compatibility and capability declarations. It does not mutate caller-owned slices. A capability change therefore changes the package identity. This digest is an integrity identity, never an approval token by itself.

`VerifyDirectory` opens a confined filesystem root, verifies the complete inventory and hashes file bytes without executing them. It rejects symlinks, nonregular files, undeclared or missing files, changed sizes/content, executable-mode mismatches and oversized directory trees. Cancellation is checked during traversal and hashing. Empty directories carry no declared content identity. This verifies the inspected bytes; it cannot freeze a directory another process can edit. The installer must revalidate bytes while copying into its own immutable installation before committing a lockfile.

Verification evidence must distinguish successful manifest parsing, verified directory content, runtime compatibility, user trust and actual extension activation. Passing one does not establish the others.

## Pinned local staging

`StageDirectory` requires the expected package digest and an existing nonsymlink `0700` staging parent outside the source directory. It verifies the source, copies only declared content through confined filesystem roots, rechecks byte hashes, sizes and executable bits during copying, and verifies the finished snapshot against the same pin. The manifest and data files become owner-readable (`0400`); executable entrypoints become owner-readable/executable (`0500`). Files and containing directories are synced before success. These permissions support private ownership but are not an OS sandbox against the owning user.

The returned `Snapshot` owns its temporary directory. `Close` removes that directory and is idempotent after successful removal; it does not alter the source. Cancellation and errors during staging remove partial output, with cleanup errors returned to the caller. A verified snapshot remains separate from installation, trust approval and activation. The future store transaction must commit the snapshot and lock metadata together and provide recovery/rollback; neither a lockfile nor power-loss qualification is implied by this staging API.

## Hand compatibility and interpreter review

`Manifest.CheckHandVersion` checks the selected Hand version against `minimum_hand` using semantic-version precedence. Prereleases sort below releases, numeric prerelease components compare numerically, and build metadata does not affect precedence. Unknown development version strings produce a diagnostic rather than assuming compatibility.

`ReviewRuntime` resolves the declared bare command through PATH, canonicalises its executable path and hashes the bounded regular executable without running it. A missing command identifies the unmet runtime requirement. The review records the exact path/content, minimum required version and host-execution boundary. Current version-probe adapters support runtime names `python`, `node` and `ruby`; other runtimes require a host adapter and fail explicitly.

`ProbeRuntime` requires a separately supplied digest matching that review. It revalidates and copies the approved executable into a private temporary directory, executes only the host-selected `--version` argument, uses a minimal environment and removes the temporary copy afterward. It never loads the package's extension source. Runtime shared libraries are not snapshotted, and this is host execution rather than an OS sandbox. Approval is required even for the version probe; manifest content or PATH discovery alone grants no execution authority.

Probes have a five-second ceiling, honour earlier cancellation, own and join their process group, and retain at most 4 KiB from each output stream. Truncated or unrecognised responses fail. The parsed version is compared to the required minimum. Review changes invalidate prior approval; changed executable bytes/mode fail before execution. These APIs still need installation/activation wiring and native interpreter qualification; the compiled regression peer establishes protocol and ownership behaviour, not universal interpreter portability.

## Reviewed package store transactions

`OpenStore` requires a known Hand semantic version and a private `0700` nonsymlink store. Its lifetime owns an exclusive `flock` writer lease; a second opener is refused. The store contains verified content under `objects/<package-digest>`, private `staging`, a writer lock and a versioned `lock.json`. The lockfile records a generation, installed package identities, selected digests and retained revisions with version/source provenance. Limits are 128 installed packages, 32 retained revisions per package and 256 KiB encoded lock metadata.

`PrepareInstall`, `PrepareRollback` and `PrepareRemove` produce concrete change reviews. The review includes store identity, action, prior generation/current digest and, where applicable, the exact source, manifest and new digest. `Apply` requires separate approval of that complete review, rechecks the prior state and package integrity/Hand compatibility, stages content and then commits a synced temporary lockfile by rename. Changed capabilities appear in the reviewed manifest and invalidate the previous approval. A review from another store or an earlier generation is refused.

Rollback selects and revalidates an explicitly retained digest. Removal drops the installed entry; it does not delete source directories or activate/deactivate a running peer. Content objects are retained pending a separate garbage-collection policy. If a content object was persisted but a later lock commit failed, it remains unreferenced and cannot become an installed package merely by existing on disk. A future collector must distinguish these objects from retained revisions.

`ChangeResult.Committed` becomes true after lockfile rename, including if a later directory sync or cleanup reports an error. Callers must inspect that field instead of assuming every error means no change. Cancellation before rename leaves the previously committed lockfile intact. This is a tested transaction boundary, not a claim of qualified power-loss recovery. Installed packages are not automatically activated; CLI workflows, runtime review and activation wiring remain separate implementation work.

## Explicit archive import

`ImportArchive` accepts a local `.zip`, `.tar`, `.tar.gz` or `.tgz` plus separate SHA-256 pins for the archive bytes and package identity. It first copies and verifies the archive into private storage, then extracts and invokes the ordinary directory verifier/stager. The returned snapshot can pass through reviewed store installation. This API does not download archives or execute their contents.

The manifest must be at archive root. Ordinary leading `./` paths and relative tar root entries are supported; wrapper directories are not stripped implicitly. Links, special files, traversal/absolute paths, duplicates and unlisted files fail. Gzip checksums are consumed and verified; nonzero data after tar termination and more than 1 MiB trailing padding fail. Archives are limited to 512 MiB compressed, 4,096 entries, 128 MiB per regular file and 512 MiB content plus the bounded manifest when expanded. ZIP central-directory metadata is capped at 8 MiB before indexing. Multipart and ZIP64 layouts are explicitly unsupported under these package limits.

Failures and cancellation clean up owned import output; successful imports leave only the returned snapshot. Use `Store.PrepareSnapshot` to carry the importer-observed original archive location and archive SHA-256 into the approval and retained revision. The temporary staging path remains separate from this provenance.

## Pinned local Git object import

`ImportGit` accepts an explicitly selected local checkout or bare repository, a full lowercase 40-character SHA-1 commit and the package SHA-256 pin. The host Git executable is required. Fixed read-only commands verify the object type, inspect committed entry modes and export tar without checking out the working tree. Symbolic refs, symlinks, submodules and special modes are refused. Replace refs, system/global Git configuration and terminal credential prompting are disabled. The process group has a 60-second operation ceiling; tree output and archive bytes are bounded.

Git archive export attributes can affect which committed files are exported, but the resulting manifest and bytes must still match the separate package pin. A single global tar comment containing a Git commit ID is tolerated as metadata only; it is not treated as evidence of provenance. Other global metadata, including path overrides, is rejected. Working files are not imported or modified, and checkout hooks are not invoked.

The resulting owned snapshot can pass through ordinary reviewed store installation. `Store.PrepareSnapshot` retains the original repository location and exact commit in the reviewed and stored provenance. Remote transport/authentication remains pending. This API is local Git object import, not a claim that remote Git installation is complete.


## Provenance and lockfile compatibility

Owned snapshots carry an `Origin` value: local source location, archive location plus archive SHA-256, or local Git repository plus full commit. `PrepareSnapshot` verifies the snapshot while holding its lifetime lock and copies this importer-provided origin into the change review. Changing origin invalidates approval. Newly retained revisions persist it independently of the staging path; rollback preserves the original revision origin. Reinstalling an already retained content digest keeps its first recorded revision provenance.

The optional `origin` field is compatible with earlier version-1 lockfiles. An absent field means the original provenance was not recorded; reading or rolling back that revision does not invent it. A plain `PrepareInstall` describes the selected local directory. Use `PrepareSnapshot` for imported archives/Git snapshots so their original source is retained. Provenance is host-supplied metadata bound into approval, not a signature from a package publisher. The CLI uses these APIs; activation workflows remain pending.

## Process-death boundary

The permanent store crash test launches a separate writer, confirms it owns the cross-process lease, and kills it either after syncing the temporary replacement before rename or after committed removal. Reopening retains the exact old lock in the former case and the committed new state in the latter. Retained package content remains verified and the dead writer's lease is released. Twenty repeated trials per case passed in the native development environment.

An unrenamed temporary lock left by process death is never read as installed state. After acquiring the exclusive writer lease and validating committed state, store opening removes bounded private regular files with Hand-generated temporary-lock names. It preserves symlinks, directories, other permissions and unrelated names, and does not clean a corrupt store. `Recovery()` reports removed temporary-lock and preserved unsafe-entry counts. Root traversal is capped at 4,096 entries before removal, and successful cleanup is directory-synced. Staging/object garbage collection and broader recovery qualification remain pending. Process death does not establish power-loss durability, filesystem fault tolerance or another platform's behaviour.

## Local package CLI

Package commands run before provider/configuration setup and do not execute installed extensions. `inspect` verifies content and reports the manifest/package digest. `list` reports the lock and recovery counts. A store defaults to `~/.hand/packages`; `--store` selects another explicit private store.

```sh
hand packages inspect --source /absolute/package
hand packages list --store /absolute/private-store
hand packages review-install --source /absolute/package --pin PACKAGE_SHA256 --store /absolute/private-store --out /absolute/install-review.json
hand packages apply --review /absolute/install-review.json --approve APPROVAL_DIGEST
hand packages review-rollback --name PACKAGE_NAME --pin RETAINED_PACKAGE_SHA256 --store /absolute/private-store --out /absolute/rollback-review.json
hand packages review-remove --name PACKAGE_NAME --store /absolute/private-store --out /absolute/remove-review.json
```

Review commands print the complete review, its path and approval digest. Read the review before supplying that digest to `apply`. Review files are newly created exclusively with mode 0600, bounded and synced; existing files are not replaced. Updates use `review-install` with a new pinned package. Apply prints `result.committed` even when it also reports an error; a committed result must not be treated as a safe blind retry. Unexpected flags/positional arguments and mismatched approval are refused.

Builds with an unknown `dev` version can inspect/list and review removal. Installation and rollback still require successful manifest compatibility against a known semantic build version; inspection does not bypass that gate. Archive/Git review commands are described below. Activation/runtime workflows remain pending. `scripts/check_package_cli.py` reproduces the local command journey against an explicitly supplied binary and example package.


## Archive and local Git review commands

```sh
hand packages review-archive --source /absolute/package.tar.gz --archive-sha256 ARCHIVE_SHA256 --pin PACKAGE_SHA256 --store /absolute/private-store --out /absolute/archive-review.json
hand packages review-git --source /absolute/local-repository --commit FULL_COMMIT_SHA1 --pin PACKAGE_SHA256 --store /absolute/private-store --out /absolute/git-review.json
hand packages apply --review /absolute/archive-review.json --approve APPROVAL_DIGEST
```

Review generation imports and verifies the selected pinned source, produces a review naming its original location and provenance, and removes the inspection snapshot. Apply re-imports the original source and verifies its pins before committing the store, so it does not depend on an abandoned temporary directory. An unavailable or changed pinned source fails without a store commit. Returning the original pinned source permits retry while the reviewed store generation is unchanged. Rollback continues to use retained verified content, without requiring the original archive/repository. Remote Git fetch and extension activation are still separate unfinished work.

## Verified installed selections

`Store.ResolveSelected` accepts 1–16 explicitly named installed packages, rejects duplicate names and verifies the selection while holding one store generation. It checks each current retained object's file integrity, manifest/package identity, version metadata and Hand compatibility. Corruption, symlinked object directories, missing packages or cancellation returns no partial selection. Returned manifests are independent values rather than references to cached store state.

`hand packages show --store /absolute/private-store --name PACKAGE_NAME` exposes this verified selection, including its generation, package digest, manifest and retained directory. It requires a compatible known Hand version; `list` remains available for metadata inspection on development builds. Resolution does not authorise execution or freeze external filesystem edits. Launch preparation must still fingerprint the selected files and obtain separate execution approval. Conversion to startup launch reviews remains pending.

Installed extension selections can be converted with `app.ReviewPackageExtensions` into the existing explicitly approved startup format. Conversion revalidates installed content and compatibility, binds stable package/extension state identities, and requires separately approved runtime reviews before interpreter version probes. It does not start extension entrypoints. All package resources, including executable helper and interpreted source files, remain fingerprinted within the launch asset quotas; a separately copied native entrypoint is omitted from assets only when no package argument references it.

The permanent installed-package journey covers actual Go and Python processes after the original installation source is deleted, exact startup-file approval, question/answer callbacks, session state across restart, and joined snapshot cleanup. This qualification currently exercises the host boundary in the development checkout. CLI selection wiring, container launch support and final platform/candidate qualification remain outstanding.

The CLI now provides the installed-package review path:

```sh
hand packages review-runtimes --names python-task-note --out runtimes.json
hand packages review-extensions --names python-task-note --workspace /absolute/project --snapshots /absolute/private/snapshots --runtime-approvals runtimes.json --out extensions.json
```

The first command resolves and fingerprints interpreters without executing them. Its output reports `review_digests` keyed by package/runtime; the written object contains a `reviews` array whose `approved_digest` fields are empty. Inspect each runtime requirement, executable and fingerprint, then explicitly enter the corresponding digest into that field to approve its version probe. The second command refuses missing or mismatched approvals, probes the approved interpreters and writes a new exclusive startup review. Neither command starts extension entrypoints. Native-only selections do not need `--runtime-approvals`.

Inspect the resulting startup review and use its reported `approval_digest` with `--extension-config extensions.json --approve-extension-config DIGEST` at startup, or the existing reviewed reload path. Startup approval is separate from runtime probe approval. Both commands accept `--store`; multiple `--names` are comma-separated in the intended launch order. Review output files are exclusive and incomplete writes are removed.

Select installed skills at startup with `--package-skills /absolute/skills.json`:

```json
{
  "version": 1,
  "store": "/absolute/private/package-store",
  "skills": [
    {"package": "verification", "digest": "<installed package SHA-256>", "path": "SKILL.md", "name": "package-verification"}
  ]
}
```

Use the real digest reported by package inspection or `packages show`. The file is explicitly selected, never discovered from a repository. Skills are verified, loaded as read-only text snapshots and added to the existing index; authored skills retain precedence. Changes to installed package content require updating the selected digest. The same selection is available to interactive, RPC and one-shot runtime construction. Reading skill text grants no execution authority. Selection files contain no extension execution approval.

Use a pinned package prompt in one-shot mode with `--package-prompt /absolute/prompt.json -p "Describe the requested change"`. Its selection file contains `version: 1`, an absolute `store`, `package`, the exact installed package `digest`, and the inventoried prompt `path`. Hand verifies the resource and supplies it as user-level instructions with package provenance. The prompt body is literal: it does not expand `@file` references, slash commands, shell expressions or template expressions. Explicit attachments in the user's `-p` text still use normal attachment checks. This flag currently requires one-shot mode; interactive/RPC prompt selection remains pending.

Build an installable native example without hand-writing its manifest:

```sh
python3 scripts/package_extension_example.py --source-root /absolute/hand-source --hand-binary /absolute/hand --example tool-viewer --out /absolute/new-build-directory
```

Supported examples are `task-note`, `tool-policy`, `verification-hook` and `tool-viewer`. The script builds with the selected Go environment, writes the executable inventory/capabilities, then uses Hand's real package inspector to obtain the content pin. Output contains `package/`, raw command evidence and `run.json`. Installation remains a separate `packages review-install`/`apply` decision. No extension is executed during packaging. Reproducing identical binaries requires the same source, dependencies and Go toolchain; this script does not publish a release.
