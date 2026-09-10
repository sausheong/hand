package checkpoints

import (
	"errors"
	"os"
)

// Unsupported operations fail before touching caller paths or acquiring authority.
// Platform selectors reference these same implementations; tests exercise them
// on supported platforms without pretending to run another operating system.
func unsupportedExchangeFiles(root *os.File, source, destination string) error {
	return errors.New("atomic restore exchange is unsupported on this platform")
}

func unsupportedMoveExclusive(root *os.File, source, destination string) error {
	return errors.New("exclusive restore rename is unsupported on this platform")
}
