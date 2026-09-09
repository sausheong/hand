package permissions

import (
	"errors"
	"os"
)

// Unsupported operations fail before touching caller paths or acquiring authority.
// Platform selectors reference these same implementations; tests exercise them
// on supported platforms without pretending to run another operating system.
func unsupportedAuthorityFile(path string) (*os.File, error) {
	return nil, errors.New("authority locking is unsupported on this platform")
}
