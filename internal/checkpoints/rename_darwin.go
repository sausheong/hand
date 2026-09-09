//go:build darwin

package checkpoints

import (
	"golang.org/x/sys/unix"
	"os"
)

func exchangeFiles(parent *os.File, from, to string) error {
	if err := renameNames(parent, from, to); err != nil {
		return err
	}
	return unix.RenameatxNp(int(parent.Fd()), from, int(parent.Fd()), to, unix.RENAME_SWAP)
}
func moveExclusive(parent *os.File, from, to string) error {
	if err := renameNames(parent, from, to); err != nil {
		return err
	}
	return unix.RenameatxNp(int(parent.Fd()), from, int(parent.Fd()), to, unix.RENAME_EXCL)
}
