# Installation, upgrades and rollback

This guide describes the current development implementation. A qualified release
candidate and a released Harness dependency are still pending. Archives built
from a dirty checkout are development artifacts even when their version output
contains a Git commit. Consult the release's acceptance evidence before choosing
it for an upgrade.

## Choose and verify an archive

Supported archive targets are `darwin-amd64`, `darwin-arm64`, `linux-amd64` and
`linux-arm64`. Choose ARM64 for Apple Silicon and Linux ARM64; choose AMD64 for
Intel/AMD x86-64. Download the archive and `SHA256SUMS` from the **same selected
release**. Retain the prior executable and its complete archive.

Before extracting, verify the selected file. Set `HAND_ARCHIVE` to its actual
filename in the download directory:

```sh
export HAND_ARCHIVE='hand-VERSION-darwin-arm64.tar.gz'
python3 - <<'PY'
import hashlib, os
from pathlib import Path
archive = Path(os.environ['HAND_ARCHIVE'])
rows = [line.split() for line in Path('SHA256SUMS').read_text().splitlines()]
matched = [digest for digest, name in rows if name == archive.name]
if len(matched) != 1 or hashlib.sha256(archive.read_bytes()).hexdigest() != matched[0]:
    raise SystemExit('Archive checksum missing, duplicated or mismatched')
print('Verified', archive.name)
PY
```

Replace `VERSION` and the target with the downloaded filename. A checksum checks
integrity relative to that manifest; obtain both through the trusted release
channel. Extract the complete verified archive into a new versioned directory
outside any project workspace. Keep these files together:

- `hand`: the native CLI.
- `hand-tool-worker-linux`: the Linux container worker for the same architecture.
- `WORKER.json`: worker digest, protocol and release identity.
- `README.md` and `LICENSE`.

Run the extracted `hand --version` and `hand --help` before changing your PATH.
These commands do not require model credentials or create session state. The
version and commit must match the selected release evidence. Point your PATH or
launcher at the versioned installation rather than overwriting the prior copy.

The CLI needs no separate language runtime. Host shell tools need Bash. MCP
servers and extensions may need additional runtimes. Container execution needs
an available Docker engine, an explicitly selected cached immutable image with
Bash, and the packaged worker. Configure absolute `execution.docker`,
`execution.socket` and `execution.worker` paths, `execution.backend: "container"`,
`execution.image` as the image's `sha256:` identity, and
`execution.worker_sha256` from `WORKER.json`. An upgrade may change the worker
hash: review and update path and hash together. A remote engine with a different
architecture is outside the archive contract.

## Preserve data before the first upgraded run

Stop Hand processes using the affected sessions and wait for their work to join.
Back up the complete `~/.hand` directory, including hidden files, alongside each
workspace's `.hand` directory and the workspace files themselves. Use a new
backup location; protect it like the original data because transcripts, tool
results and local configuration may contain sensitive content. Record the old
binary version, workspace absolute path and backup location.

Sessions live under `~/.hand/sessions`; their catalogue is bound to the workspace
path. Keep the complete session store, including attachments and migration
backups. Copying only one JSONL file is not a complete backup. Opening an existing
session can migrate its durable format; the migration preserves an original
backup. Keep your independent pre-upgrade backup as well.

Run the new binary from the original workspace. `/resume` lists its sessions;
`/resume SESSION-ID` selects one. `/new` and `--new-session` create a new session
without deleting earlier history. Check the selected session and transcript
before asking the agent to change files. Legacy permission proposals require
separate review; follow [permission migration](permission-migration.md).

For a portable session export without starting a model, obtain its stable ID
from `/resume`, stop the process holding it, and run:

```sh
hand --session SESSION-ID --export-session /absolute/new-path/session.jsonl
```

The destination must be new. Preserve the sibling attachment directory produced
with the export. Export is a supported archival format, not a promise that an
older Hand binary can import or write it.

## Roll back without downgrading the only data copy

Stop the upgraded binary and preserve its current data separately, including any
new sessions or exports. Restore the pre-upgrade `~/.hand` and workspace settings
from backup, retain the new data in its separate location, then select the old
binary. Restore workspace files only when that is the intended rollback; session
rollback does not reverse file edits, shell commands or external effects.

Do not run an older binary against the sole upgraded session store. Older
versions are not guaranteed to understand newer schemas or scoped grants. Do
not bypass writer ownership by deleting locks, or repair a journal by removing
individual records. A future-schema or corruption error needs the matching
supported version or recovery from a preserved backup.

## Build and inspect development archives

A source build needs Go 1.25.1 or newer and Python 3. The distribution builder
requires a new output directory and leaves failed output intact for inspection:

```sh
make dist VERSION=development DIST_DIR=/absolute/new-build-directory
python3 scripts/smoke_dist.py --dist /absolute/new-build-directory \
  --version development --commit "$(git rev-parse HEAD)" --require-worker
```

The smoke runner checks all four archives and worker manifests and executes the
CLI matching its host. Its other target rows explicitly report that their
binaries were not run. Cross-compilation does not prove runtime qualification.
`make release` tags and pushes, triggering publication; it is a separate action
from building and checking a candidate.
