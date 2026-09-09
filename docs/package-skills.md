# Installed package skills and relative resources

Installed skills are selected explicitly. Installing a package alone does not load its guidance or execute its code. Use the existing package review/install commands to install and inspect a pinned package, then select its declared skill files with `--package-skills`:

```json
{
  "version": 1,
  "store": "/absolute/path/to/package-store",
  "skills": [
    {
      "package": "your-installed-package",
      "digest": "the-installed-package-sha256",
      "path": "skills/review/SKILL.md",
      "name": "package-review"
    }
  ]
}
```

Replace the store, package, digest and path with values from your installation. The digest must be the complete lowercase 64-character package SHA-256. Existing project/personal skills retain precedence when a selected package skill uses the same name. A missing, incompatible, corrupt or differently pinned package fails selection rather than replacing the current skill provider.

```sh
hand --package-skills /absolute/path/to/selection.json
```

The selection is also applied before RPC and one-shot execution. This startup file is an explicit configuration input; Hand does not discover or execute it merely because a repository contains it.

## Read the skill and its declared resources

The model first calls:

```json
{"name":"package-review"}
```

on `load_skill`. The returned body identifies its package digest, retained source path and filesystem resource base directory. Context inspection also reports the retained skill source.

For an instruction referring to `refs/checklist.md`, call the same tool with:

```json
{"name":"package-review","resource":"refs/checklist.md"}
```

The resource is resolved relative to the selected skill's directory within the pinned package. A reference such as `../shared/checklist.md` is valid only if it stays inside the package and the resolved file is declared in that revision's manifest. Absolute paths, escapes, undeclared files, symlinks, invalid UTF-8 and resources over 64 KiB are rejected. The returned bytes must match the declared size and SHA-256 on every read; a modified file produces an error without exposing its content.

This is a bounded data read through the selected package loader. It does not execute scripts, run extensions, grant network access, or expand general filesystem access. A resource file containing code is returned as text only. Ordinary `read_file` remains restricted by its workspace/backend configuration. Authored skills retain their existing resource-reading behaviour; a package resource cannot shadow an authored skill with the same name.

Selected bodies and resource descriptors stay bound to their selected revision. Updating or removing the installation does not redirect an existing selection to a different revision: the package store retains its content-addressed objects. Selecting an updated package requires its new digest. Reselection of a removed package is rejected. Runtime reselection and session persistence are not added by this change.

## Development verification

The package/application tests cover nested source paths, source removal, package removal, authored precedence, invalid pins, traversal, undeclared resources and tampered bytes. `scripts/check_package_text_cli.py` exercises the built client against a local fixture provider: install a pinned package, delete its original source, load the skill, read its relative text resource, preserve literal prompt text and reject a wrong pin before a provider request.

```sh
python3 scripts/check_package_text_cli.py \
  --hand-binary /absolute/path/to/hand \
  --out /absolute/path/to/new-evidence-directory
```

This runner uses an isolated fixture home and `--yes` for its scripted tool requests. It makes no paid model calls and does not establish live-model performance or full release qualification. Run it with a development/release binary whose version satisfies the fixture manifest, and inspect its raw requests, commands and result file. The current source still requires the local Harness development workspace described in `docs/acceptance/development-integration.md` until a reviewed Harness release is pinned.

### Container execution

Explicitly selected package bodies and declared text resources remain available
through the bounded package loader when process execution uses the container
backend. This does not mount the host package store into the container or make
ordinary filesystem tools unrestricted. Non-package skill loads retain their
configured backend, and deselecting package skills restores that loader.

The journey runner accepts `--execution-config /absolute/path/to/config.json`
for a pinned container fixture. It requires a read-only, network-disabled
configuration, verifies resource loading, runs `uname -s` through the process
tool, and checks that no new Harness containers survive completion. On the
verified macOS host, the process returned Linux. The fixture approves this
harmless command with `--yes`; it makes no paid model calls. This is one
integration journey, not complete container or release qualification.
