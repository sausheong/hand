# Output-load ownership

Opening a truncated result starts a model-owned retrieval worker. Closing or replacing the viewer cancels it. Completion is published only after the worker has exited. The UI discards stale results, and application shutdown cancels and joins all retained workers even if their Tea commands were never consumed.

At most four unfinished loads may exist while cancelled readers stop. Further requests show a retry message. Finished workers are reclaimed on result delivery or the next request. Controller lock acquisition checks cancellation while waiting, so a configuration operation cannot prevent delivery of cancellation to a waiting reader.

Tests cover shutdown joining, ignored result delivery, capacity enforcement and cancellation after entering an actual controller-lock wait. The TUI suite passed before test-only additions; focused lifecycle tests then passed. Vet passed. Raw results and aggregate source hashes are in `output-lifecycle-integration.json`.

Session snapshot and individual JSON decode operations are not preemptible mid-operation. Native worst-case cancellation qualification remains pending, as do external output-artifact retrieval and the other M3.3 requirements. No acceptance group is closed.
