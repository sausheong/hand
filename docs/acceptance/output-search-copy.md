# Search and copy captured output

Open `/output [result number]`, press `/`, enter a literal search string and press Enter. Search is case-sensitive, accepts up to 256 characters and finds matching logical lines across display wrapping. `n` and `N` move forward and backward through matching lines, wrapping at either end. Esc cancels search entry. The status line reports no matches explicitly.

Press `y` to request copying the full sanitised output, including any error text, through OSC52. Hand generates and base64-encodes this sequence; tool-provided escape sequences are removed before copying. This works only in terminals that permit OSC52 clipboard writes. Hand reports that the request was sent, not that the clipboard accepted it. Write errors are visible. Output over 8 MiB is rejected explicitly rather than truncated; use session export for larger records.

Controls and status occupy two reserved rows. The viewer reflows and recomputes matching rows on resize. Copy completion is tied to the initiating viewer so stale results cannot update another view.

The TUI regression suite and vet passed. Native clipboard acceptance, dedicated diff viewing, captured-to-file expansion, full transcript migration and performance qualification remain pending. See `output-search-copy-integration.json` for raw development evidence and aggregate patch hashes.
