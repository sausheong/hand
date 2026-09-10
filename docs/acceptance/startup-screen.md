# Cancellable interactive startup screen

Interactive startup now opens a Bubble Tea connection-status screen before waiting for MCP construction. Esc, Ctrl+C and q cancel construction; the screen remains until cleanup joins. A successful Runtime transfers to the main Hand UI. Terminal errors and cancellation close any returned Runtime before releasing its owning session and registry. No-MCP startup keeps its direct construction path; one-shot mode keeps stderr progress and bounded construction.

Permanent tests cover visible pending/cancelling state, joined cancellation, terminal failure before Init and successful ownership transfer. CLI package race-suite raw events and source hashes are saved in `startup-screen-integration.json`; vet passed. These are development checks, not native terminal qualification.

This intermediate implementation does not complete the startup requirement. The main editor is still unavailable during construction, connections remain sequential, and retry controls are not implemented. M3.4 stays in progress. The aggregate patch excludes the local go.mod replacement and requires unpublished Harness candidate `fd9ee98d2bef92fee2658eabc5b4da49038fcc22`.
