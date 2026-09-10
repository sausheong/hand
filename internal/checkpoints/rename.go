package checkpoints

import (
	"errors"
	"os"
	"strings"
)

// renameNames confines native rename operations to two leaf entries in an
// already-open directory. Callers own and retain that directory descriptor.
// These primitives do not follow a final symlink or delete displaced content.
func renameNames(parent *os.File, from, to string) error {
	if parent == nil {
		return errors.New("restore parent directory unavailable")
	}
	for _, name := range []string{from, to} {
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\x00") {
			return errors.New("restore rename requires leaf names")
		}
	}
	if from == to {
		return errors.New("restore rename requires distinct entries")
	}
	info, err := parent.Stat()
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("restore parent is not a directory")
	}
	return nil
}
