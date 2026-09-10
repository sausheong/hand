# Steering attachment snapshots — development evidence

Steering now resolves file and image attachments before claiming queued input. Accepted snapshots survive retries without rereading changed files. Failed resolution leaves input editable. Durable reconciliation verifies persisted images before removing queued input. Copies prevent callers from mutating retained image bytes.

Harness rejects mismatched image counts, MIME types, sizes and digests before hydration; matching references still require verified blob reads.

The uncached race suites passed: Hand 639 and Harness 921 test/subtest events. Focused steering suites each passed 220 events across 20 repetitions. Both vet commands exited zero. These counts are development tests, not accepted requirement scenarios. Harness example packages with no tests retain their package-level skip status in raw evidence.

The aggregate patch excludes the temporary go.mod replacement and remains staged pending released Harness integration. Native platform and final acceptance evidence remain outstanding. No requirement group is marked complete.
