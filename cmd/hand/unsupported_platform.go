package main

import (
	"errors"
	"io"
	"os"
)

// Unsupported operations fail before touching caller paths or acquiring authority.
// Platform selectors reference these same implementations; tests exercise them
// on supported platforms without pretending to run another operating system.
func unsupportedLegacySettings(root *os.Root) (*os.File, error) {
	return nil, errors.New("safe legacy settings inspection unavailable on this platform")
}

func unsupportedStdioRPC() (io.ReadWriteCloser, error) {
	return nil, errors.New("interruptible RPC stdio requires a supported Linux or macOS platform")
}
