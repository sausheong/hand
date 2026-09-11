package main

import "strings"

// executionBoundaryNotice is written before the full-screen UI starts. Keep
// host mode explicit: the terminal restores after exit, so this is often the
// first line a person sees then.
func executionBoundaryNotice(boundary string) string {
	if strings.TrimSpace(boundary) == "unrestricted host" {
		return "hand: Bash commands run directly on this computer (host mode; no container isolation)"
	}
	return "hand: execution boundary: " + boundary
}
