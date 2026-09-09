// Package tui is Hand's Bubble Tea terminal UI.
package tui

import (
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/harness/compaction"
)

// compactResultMsg releases the operation only after its worker has returned.
// Cancellation does not permit another operation to race a still-exiting worker.
type compactResultMsg struct {
	usage      sessionio.UsageSummary
	usageKnown bool
	usageErr   error
	identity   agentio.RunIdentity
	generation uint64
	result     compaction.Result
	err        error
}
