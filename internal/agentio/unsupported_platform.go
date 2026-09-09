package agentio

import (
	"errors"
	"os"
)

// Unsupported operations fail before touching caller paths or acquiring authority.
// Platform selectors reference these same implementations; tests exercise them
// on supported platforms without pretending to run another operating system.
func unsupportedApprovalSnapshot(path string) (*os.File, error) {
	return nil, errors.New("safe approval snapshots unavailable on this platform")
}
