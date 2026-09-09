# Current development checkout

The verified 437-file implementation aggregate has been applied to the primary
Hand checkout. Continue source changes here, not in the earlier staging directory.
`primary-integration.json` records every applied file hash, the original aggregate,
backup location, dependency graph and verification logs. Historical cumulative
patches in this directory are evidence of development stages; do not apply them
again to the integrated checkout.

The local `go.work` selects this module and the unreleased Harness checkout at
`/private/tmp/hand-harness-persistence-20260908`. It is a development setup, not a
portable release dependency. The recorded Harness base and cumulative patch can
reconstruct that dependency; preserve its checkout until release integration.
Normal Go commands in this checkout use the workspace automatically. The initial
workspace dependency/checksum lookups were completed, and package/extension race
tests, whole-Hand build and whole-Hand vet passed. Skipped native cases do not
become passes through this integration.

Before freezing the final candidate, release the reviewed Harness changes with
appropriate approval, update Hand's go.mod/go.sum to that actual released version,
remove the machine-local go.work/go.work.sum, and rerun qualification with GOWORK=off.
The current go.mod still declares Harness v0.3.9; that release does not contain the
new APIs. No clean-candidate or final-acceptance claim follows from this local
integration. Preserve unrelated changes while preparing the final commit.
