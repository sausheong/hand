package checkpoints

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"syscall"
)

type RestoreEvent struct {
	Phase        string  `json:"phase"`
	Path         string  `json:"path"`
	Operation    string  `json:"operation"`
	RecoveryName string  `json:"recovery_name"`
	Expected     *Record `json:"expected,omitempty"`
	Restored     *Record `json:"restored,omitempty"`
}
type RestoreResult struct {
	Applied      bool
	RecoveryPath string
}

// restoreFile is the transaction executor for one already-reviewed action.
// record must durably persist intent before returning nil. Displaced content is
// retained for recovery; retirement belongs to a later committed transaction.
// Callers must hold workspace mutation ownership and revalidate review scope.
func restoreFile(ctx context.Context, workspace *os.Root, a RestoreAction, before *Snapshot, record func(RestoreEvent) error) (result RestoreResult, err error) {
	if workspace == nil || record == nil || a.Conflict != "" || !fs.ValidPath(a.Path) || a.Path == "." || strings.ContainsRune(a.Path, 0) {
		return result, errors.New("invalid restore action")
	}
	if err = validateSnapshot(before); err != nil {
		return result, err
	}
	if omittedAt(before, a.Path) {
		return result, errors.New("restore path outside captured scope")
	}
	if a.Expected != nil && (a.Expected.Path != a.Path || a.Expected.Kind != "regular" || a.Expected.Mode > 0777 || a.Expected.Size < 0 || a.Expected.Size > 128<<20 || !validDigest(a.Expected.Hash)) {
		return result, errors.New("invalid expected restore record")
	}
	old, hadOld := recordsByPath(before)[a.Path]
	if (a.Restore != nil) != hadOld || (hadOld && old != *a.Restore) {
		return result, errors.New("restore target differs from before snapshot")
	}
	switch a.Operation {
	case "create":
		if a.Expected != nil || a.Restore == nil {
			return result, errors.New("invalid create action")
		}
	case "replace":
		if a.Expected == nil || a.Restore == nil {
			return result, errors.New("invalid replace action")
		}
	case "remove":
		if a.Expected == nil || a.Restore != nil {
			return result, errors.New("invalid remove action")
		}
	default:
		return result, errors.New("invalid restore operation")
	}
	parent, err := restoreParent(workspace, path.Dir(a.Path))
	if err != nil {
		return result, err
	}
	defer parent.Close()
	dir, err := parent.Open(".")
	if err != nil {
		return result, err
	}
	defer dir.Close()
	leaf := path.Base(a.Path)
	if err = matchRestoreFile(ctx, parent, leaf, a.Expected); err != nil {
		return result, err
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return result, err
	}
	stage := ".hand-restore-" + hex.EncodeToString(nonce[:])
	keep := false
	defer func() {
		if !keep {
			parent.Remove(stage)
		}
	}()
	if a.Restore != nil {
		content, ok := before.Content(a.Restore.Hash)
		if !ok {
			return result, errors.New("restore content missing")
		}
		f, e := parent.OpenFile(stage, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return result, e
		}
		for len(content) > 0 {
			if e = ctx.Err(); e != nil {
				f.Close()
				return result, e
			}
			n := min(len(content), 32<<10)
			written, e := f.Write(content[:n])
			if e != nil {
				f.Close()
				return result, e
			}
			if written != n {
				f.Close()
				return result, io.ErrShortWrite
			}
			content = content[n:]
		}
		if e = f.Chmod(os.FileMode(a.Restore.Mode)); e != nil {
			f.Close()
			return result, e
		}
		if e = errors.Join(f.Sync(), f.Close()); e != nil {
			return result, e
		}
	}
	// The staging directory entry must survive alongside the durable intent.
	if err = dir.Sync(); err != nil {
		return result, err
	}
	event := RestoreEvent{Phase: "prepared", Path: a.Path, Operation: a.Operation, RecoveryName: stage, Expected: a.Expected, Restored: a.Restore}
	if err = record(event); err != nil {
		return result, err
	}
	keep = true // once intent exists, recovery owns any staging entry
	result.RecoveryPath = path.Join(path.Dir(a.Path), stage)
	if err = ctx.Err(); err != nil {
		return result, err
	}
	// Detect edits since preparation before changing directory entries.
	if err = matchRestoreFile(ctx, parent, leaf, a.Expected); err != nil {
		return result, err
	}
	switch a.Operation {
	case "create":
		err = moveExclusive(dir, stage, leaf)
	case "replace":
		err = exchangeFiles(dir, stage, leaf)
	case "remove":
		err = moveExclusive(dir, leaf, stage)
	}
	if err != nil {
		return result, err
	}
	result.Applied = true
	if err = dir.Sync(); err != nil {
		return result, fmt.Errorf("restore directory durability uncertain: %w", err)
	}
	// An exchange captures the actual entry at mutation time. If it changed in
	// the final race window, retain it and report uncertainty for reconciliation.
	if a.Expected != nil {
		if err = matchRestoreFile(ctx, parent, stage, a.Expected); err != nil {
			return result, fmt.Errorf("displaced file requires recovery: %w", err)
		}
	}
	if a.Restore != nil {
		if err = matchRestoreFile(ctx, parent, leaf, a.Restore); err != nil {
			return result, fmt.Errorf("restored file changed before completion: %w", err)
		}
	}
	event.Phase = "applied"
	if err = record(event); err != nil {
		return result, err
	}
	if a.Operation == "create" {
		result.RecoveryPath = ""
	}
	return result, nil
}
func matchRestoreFile(ctx context.Context, root *os.Root, name string, want *Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := root.Lstat(name)
	if want == nil {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return errors.New("restore destination exists")
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != os.FileMode(want.Mode) || info.Size() != want.Size {
		return errors.New("restore file type, mode or size conflict")
	}
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		return errors.New("restore file identity changed")
	}
	h := sha256.New()
	buf := make([]byte, 32<<10)
	reader := io.LimitReader(f, want.Size+1)
	var n int64
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		count, e := reader.Read(buf)
		n += int64(count)
		h.Write(buf[:count])
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
	}
	final, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if n != want.Size || hex.EncodeToString(h.Sum(nil)) != want.Hash || !os.SameFile(opened, final) || final.Mode() != opened.Mode() || !final.ModTime().Equal(opened.ModTime()) {
		return errors.New("restore content or identity conflict")
	}
	return nil
}
