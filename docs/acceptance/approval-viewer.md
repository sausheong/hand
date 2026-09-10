# Approval previews and separated diff hunks

Pending approvals reserve at most eight rows at 80×24, including decision controls. Long previews show a visible expansion hint. Press `v` to open the complete available preview; use arrows, Page Up/Page Down, Home/End, `/` search and `n`/`N` navigation. Escape returns to the approval decision. Keys inside the viewer do not approve the tool. `y` copies the preview through the existing explicit OSC52 path; approve only after leaving the viewer.

File previews identify the requested path. Distant changes now form separate unified hunks instead of displaying the entire intervening region as deleted and re-added. Terminal sanitisation applies before rendering, including tool names and paths. Long tool names are shortened in the panel so decision keys remain visible; the viewer includes the full name.

Existing preview limits remain explicit: changes over 4,000 combined lines or 2 MiB combined text are labelled as omitted. Full inspection of changes beyond these limits remains required work. Native terminal qualification, all transcript block migration, incremental rendering and performance acceptance are also pending.

See `approval-viewer-integration.json` for development test evidence and aggregate patch hashes. The patch remains staged pending released Harness integration.
