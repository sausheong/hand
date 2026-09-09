# Captured stdout and stderr viewer

Use `/output [result number] stdout` or `/output [result number] stderr` to open a captured stream. Without a number, the latest retained result is selected. The viewer supports scrolling, search and explicit copy requests through the existing controls.

Hand resolves the selected session/tool-call identity, obtains its persisted artifact reference and reads it through the configured Harness capture store. It never opens a filename parsed from output prose. Missing references, expired files, digest mismatches, active captures and binary data are visible errors; the previous preview remains available. A verified capture that reached its size limit is explicitly labelled as a captured prefix.

The default reader uses Harness's per-user temporary output store. Embedders using a custom `BashTool.OutputStore` must supply the same store through `Controller.OutputStore` before use. Session replay and session switching retain the identities needed for lookup. Loads use model-owned cancellation and shutdown joining.

Regression coverage reopens a session, expands stdout, reaches the capture tail, removes the file and verifies the missing-artifact error, then requests absent stderr. This is staged integration against Harness candidate 698a0c2. Native qualification, portable export/fork of process artifacts and remaining M3.3 work are pending. Exact test counts and raw evidence are recorded in `artifact-viewer-integration.json`.
