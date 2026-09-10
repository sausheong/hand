//go:build darwin || linux

package main

import (
	"os"
	"syscall"
)

func openLegacySettings(root *os.Root) (*os.File, error) {
	return root.OpenFile(".hand/settings.json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}
