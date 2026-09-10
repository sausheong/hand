# Full captured tool output

The staged terminal integration retains complete tool-result strings alongside their previews. `/output` opens the latest result; `/output 2` opens the second retained result. Use arrows, Page Up/Page Down, Home and End to scroll. Esc or q returns to the conversation. The viewer reflows when resized and reserves space for its controls.

Successful and failed results retain their output. Display sanitisation removes terminal control sequences on both live and replay paths. Clearing the screen also clears its output list; changing sessions replaces the list with that session's replayed results.

This is the first M3.3 increment. Search/copy, a dedicated diff viewer, loading full captured-to-file artifacts and migration of the other transcript block types remain pending. The existing transcript still wraps its entire history; the 10,000-block performance gate has not been met. This patch remains staged pending released Harness integration.
