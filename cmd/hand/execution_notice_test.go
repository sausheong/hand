package main

import "testing"

func TestExecutionBoundaryNoticeMakesHostModePlain(t *testing.T) {
	const want = "hand: Bash commands run directly on this computer (host mode; no container isolation)"
	if got := executionBoundaryNotice("unrestricted host"); got != want {
		t.Fatalf("host notice = %q, want %q", got, want)
	}
}

func TestExecutionBoundaryNoticePreservesDetailedBoundary(t *testing.T) {
	const boundary = "Docker container; model-provider requests remain on host"
	if got, want := executionBoundaryNotice(boundary), "hand: execution boundary: "+boundary; got != want {
		t.Fatalf("container notice = %q, want %q", got, want)
	}
}
