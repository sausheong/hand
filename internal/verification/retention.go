package verification

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
)

type RetentionLimits struct {
	MaxRecords int
	MaxBytes   int64
}

func DefaultRetentionLimits() RetentionLimits {
	return RetentionLimits{MaxRecords: 1000, MaxBytes: 256 << 20}
}
func (l RetentionLimits) Validate() error {
	if l.MaxRecords <= 0 || l.MaxRecords > 10000 || l.MaxBytes <= 0 || l.MaxBytes > 8<<30 {
		return errors.New("invalid verification retention limits")
	}
	return nil
}

const evidenceLockName = ".verification.lock"

func lockEvidence(root *os.Root) (*os.File, error) {
	f, err := root.OpenFile(evidenceLockName, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		f.Close()
		return nil, errors.New("verification lock must be a private regular file")
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.New("verification evidence directory is busy")
	}
	return f, nil
}

// capacity counts orphan staging bytes as retained storage. It never silently
// evicts prior evidence or removes unknown files to make space.
func evidenceCapacity(ctx context.Context, root *os.Root, limits RetentionLimits, size int64) error {
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	entries, records := 0, 0
	var bytes int64
	for {
		batch, readErr := dir.ReadDir(128)
		for _, entry := range batch {
			if err := ctx.Err(); err != nil {
				return err
			}
			entries++
			if entries > limits.MaxRecords+129 {
				return errors.New("verification directory entry capacity reached")
			}
			name := entry.Name()
			info, err := root.Lstat(name)
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
				return errors.New("verification directory contains unsafe entry")
			}
			if name == evidenceLockName {
				continue
			}
			if strings.HasSuffix(name, ".json") && validID(strings.TrimSuffix(name, ".json")) {
				records++
			}
			bytes += info.Size()
			if bytes > limits.MaxBytes-size {
				return errors.New("verification evidence byte capacity reached")
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if records >= limits.MaxRecords {
		return errors.New("verification evidence record capacity reached")
	}
	return nil
}
