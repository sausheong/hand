package packages

import (
	"errors"
	"io"
	"regexp"

	"github.com/sausheong/hand/extension/protocol"
)

type RecoveryReport struct {
	RemovedTemporaryLocks  int `json:"removed_temporary_locks"`
	PreservedUnsafeEntries int `json:"preserved_unsafe_entries"`
}

var temporaryLockName = regexp.MustCompile(`^lock-[A-Z2-7]{26,128}\.tmp$`)

func (s *Store) Recovery() RecoveryReport { return s.recovery }

// Called only after obtaining the exclusive writer lease and validating the
// committed lock. No uncommitted JSON is replayed or promoted to installed state.
func (s *Store) recoverTemporaryLocks() (report RecoveryReport, err error) {
	directory, err := s.root.Open(".")
	if err != nil {
		return report, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(4097)
	if err != nil && err != io.EOF {
		return report, err
	}
	if len(entries) > 4096 {
		return report, errors.New("package store root exceeds recovery entry quota")
	}
	for _, entry := range entries {
		if !temporaryLockName.MatchString(entry.Name()) {
			continue
		}
		info, e := s.root.Lstat(entry.Name())
		if e != nil {
			return report, e
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > protocol.MaxFrameBytes {
			report.PreservedUnsafeEntries++
			continue
		}
		if e = s.root.Remove(entry.Name()); e != nil {
			return report, e
		}
		report.RemovedTemporaryLocks++
	}
	if report.RemovedTemporaryLocks > 0 {
		err = directory.Sync()
	}
	return report, err
}
