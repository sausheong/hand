# Complete persisted tool results

When an application event contains truncated display details, `/output` now retrieves the complete result from the selected session history using the session ID and tool-call ID. The lookup rejects missing, changed or ambiguous identities. Errors remain visible alongside the truncated capture; successful retrieval replaces the viewer content with the complete stored output and error text.

Viewer loads run as commands and are tied to the initiating viewer. Results arriving after another viewer opens cannot overwrite it. A new regression exercises production controller setup, loads a result larger than the event display limit and reaches its final line.

This retrieves session records, not external output files named by tools. Artifact trust, bounded file retrieval and retention failures still need implementation. Output-load cancellation/joining and native UI performance also remain pending.

The application race suite passed in the combined run. The TUI race suite passed after correcting its test controller setup. Raw failures, including the initial sandbox restriction on a local HTTP fixture, are preserved in `full-result-integration.json`; they are not counted as passes. Vet passed before the test-only controller-setup correction. The aggregate patch remains staged against the unpublished Harness candidate.
