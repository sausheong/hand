# Development evidence

These records are development observations, not final acceptance.

Archive Go source snapshots with a non-source suffix such as `.go.txt` or in a compressed archive. A `.go` copy under docs is discoverable by `go test ./...` and may create an unintended package. Preserve the original source bytes/hash and record the archive path in the evidence manifest. Never exclude production code or required tests to accommodate an archive.
