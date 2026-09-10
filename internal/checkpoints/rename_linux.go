//go:build linux

package checkpoints

import (
	"golang.org/x/sys/unix"
	"os"
)

func exchangeFiles(parent *os.File, from, to string) error {
	if err := renameNames(parent, from, to); err != nil {
		return err
	}
	return unix.Renameat2(int(parent.Fd()), from, int(parent.Fd()), to, unix.RENAME_EXCHANGE)
}
func moveExclusive(parent *os.File, from, to string) error {
	if err := renameNames(parent, from, to); err != nil {
		return err
	}
	return unix.Renameat2(int(parent.Fd()), from, int(parent.Fd()), to, unix.RENAME_NOREPLACE)
}
