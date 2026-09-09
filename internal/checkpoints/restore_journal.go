package checkpoints

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"syscall"
)

const restoreJournalName = "restore-journal.jsonl"
const restoreJournalLimit = 8 << 20
const restoreJournalRecords = 512

type restoreJournalRecord struct {
	Version   int          `json:"version"`
	Workspace string       `json:"workspace"`
	Event     RestoreEvent `json:"event"`
}

func validateRestoreEvent(e RestoreEvent) error {
	if !fs.ValidPath(e.Path) || e.Path == "." || len(e.Path) > 4096 || !strings.HasPrefix(e.RecoveryName, ".hand-restore-") {
		return errors.New("invalid restore journal path")
	}
	suffix := strings.TrimPrefix(e.RecoveryName, ".hand-restore-")
	id, err := hex.DecodeString(suffix)
	if err != nil || len(id) != 16 || suffix != strings.ToLower(suffix) {
		return errors.New("invalid restore recovery identity")
	}
	if e.Phase != "prepared" && e.Phase != "applied" && e.Phase != "cancelled" {
		return errors.New("invalid restore phase")
	}
	for _, r := range []*Record{e.Expected, e.Restored} {
		if r != nil && (r.Path != e.Path || r.Kind != "regular" || !validDigest(r.Hash) || r.Size < 0 || r.Size > 128<<20 || r.Mode > 0777) {
			return errors.New("invalid restore file record")
		}
	}
	switch e.Operation {
	case "create":
		if e.Expected != nil || e.Restored == nil {
			return errors.New("invalid restore creation")
		}
	case "replace":
		if e.Expected == nil || e.Restored == nil {
			return errors.New("invalid restore replacement")
		}
	case "remove":
		if e.Expected == nil || e.Restored != nil {
			return errors.New("invalid restore removal")
		}
	default:
		return errors.New("invalid restore operation")
	}
	return nil
}
func appendRestoreEvent(events []RestoreEvent, e RestoreEvent) ([]RestoreEvent, error) {
	if err := validateRestoreEvent(e); err != nil {
		return nil, err
	}
	var previous *RestoreEvent
	for i := range events {
		if events[i].RecoveryName == e.RecoveryName {
			previous = &events[i]
		}
	}
	if e.Phase == "prepared" {
		if previous != nil {
			return nil, errors.New("duplicate restore preparation")
		}
	} else {
		if previous == nil || previous.Phase != "prepared" {
			return nil, errors.New("restore resolution without unique preparation")
		}
		a, b := *previous, e
		a.Phase = ""
		b.Phase = ""
		if !reflect.DeepEqual(a, b) {
			return nil, errors.New("restore intent changed")
		}
	}
	return append(events, e), nil
}
func (s *Store) readRestoreJournal(ctx context.Context) ([]RestoreEvent, error) {
	f, err := s.root.OpenFile(restoreJournalName, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > restoreJournalLimit {
		return nil, errors.New("invalid restore journal")
	}
	reader := bufio.NewReaderSize(io.LimitReader(f, restoreJournalLimit+1), 16<<10)
	var events []RestoreEvent
	for {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		line, e := reader.ReadSlice('\n')
		if e == io.EOF && len(line) == 0 {
			break
		}
		if e != nil {
			return nil, errors.New("truncated or oversized restore journal record")
		}
		if len(events) >= restoreJournalRecords {
			return nil, errors.New("restore journal record limit")
		}
		var r restoreJournalRecord
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		if err = dec.Decode(&r); err != nil {
			return nil, err
		}
		var extra any
		if err = dec.Decode(&extra); err != io.EOF {
			return nil, errors.New("trailing restore record")
		}
		if r.Version != 1 || r.Workspace != s.workspace {
			return nil, errors.New("restore journal workspace/version mismatch")
		}
		events, err = appendRestoreEvent(events, r.Event)
		if err != nil {
			return nil, err
		}
	}
	return events, nil
}
func (s *Store) RestoreEvents(ctx context.Context) ([]RestoreEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ready(); err != nil {
		return nil, err
	}
	return s.readRestoreJournal(ctx)
}

// RecordRestore returns success only after append and directory durability.
// Ambiguous write failures poison the handle; replay is required on reopen.
func (s *Store) RecordRestore(e RestoreEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recordRestore(e)
}

// recordRestore requires s.mu.
func (s *Store) recordRestore(e RestoreEvent) error {
	if err := s.ready(); err != nil {
		return err
	}
	events, err := s.readRestoreJournal(context.Background())
	if err != nil {
		return err
	}
	if len(events) >= restoreJournalRecords {
		return errors.New("restore journal capacity reached")
	}
	if _, err = appendRestoreEvent(events, e); err != nil {
		return err
	}
	line, err := json.Marshal(restoreJournalRecord{1, s.workspace, e})
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if len(line) > 16<<10 {
		return errors.New("restore journal record too large")
	}
	_, total, err := s.inventory()
	if err != nil {
		return err
	}
	if total+int64(len(line)) > s.limits.MaxBytes {
		return errors.New("checkpoint store byte capacity reached")
	}
	f, err := s.root.OpenFile(restoreJournalName, os.O_CREATE|os.O_WRONLY|os.O_APPEND|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size()+int64(len(line)) > restoreJournalLimit {
		f.Close()
		return errors.New("invalid or full restore journal")
	}
	n, writeErr := f.Write(line)
	if writeErr == nil && n != len(line) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = s.syncFile(f)
	}
	err = errors.Join(writeErr, f.Close())
	if err != nil {
		s.failed = fmt.Errorf("restore journal durability uncertain: %w", err)
		return s.failed
	}
	return s.syncDirectory()
}
