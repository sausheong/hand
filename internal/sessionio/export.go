package sessionio

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sausheong/harness/session"
)

const MaxSessionExportBytes int64 = 256 << 20

// Export copies the durable graph verbatim, including identity and selection
// controls. Caller must hold application ownership and the session writer lease.
// Destination publication is exclusive; a failed copy never exposes a partial file.
func (m *Manager) Export(ctx context.Context, sess *session.Session, key, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot, err := m.catalogue.Snapshot()
	if err != nil {
		return err
	}
	var source string
	for _, record := range snapshot.Sessions {
		if sess != nil && record.ID == sess.ID && record.StoreKey == key {
			source, err = m.SessionPath(record)
			if err != nil {
				return err
			}
			break
		}
	}
	if source == "" {
		return errors.New("export session is outside workspace catalogue")
	}
	if err := sess.Flush(); err != nil {
		return err
	}
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("export source is not a regular file")
	}
	if info.Size() > MaxSessionExportBytes {
		return errors.New("session exceeds export size limit")
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	current, err := in.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(info, current) {
		return errors.New("export source changed before opening")
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return err
	}
	refs, err := session.AttachmentReferences(sess.Entries())
	if err != nil {
		return err
	}
	total := info.Size()
	for _, ref := range refs {
		if ref.Size > MaxSessionExportBytes-total {
			return errors.New("session and attachments exceed export size limit")
		}
		total += ref.Size
	}
	// Immutable sidecars are durable before the exclusive JSONL publication.
	// Failed exports may leave reusable blobs, never a partial successful JSONL.
	if err := m.store.CopyAttachments(ctx, m.agent, session.NewStore(filepath.Dir(destination)), "", sess.Entries()); err != nil {
		return err
	}
	out, err := os.CreateTemp(filepath.Dir(destination), ".hand-export-")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	defer out.Close()
	if err := copySessionExport(ctx, out, in, MaxSessionExportBytes, session.MaxSessionRecordBytes); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.exportCheckpoint != nil {
		if err := m.exportCheckpoint("before_publish"); err != nil {
			return err
		}
	}
	if err := os.Link(out.Name(), destination); err != nil {
		return fmt.Errorf("publish export without overwrite: %w", err)
	}
	if m.exportCheckpoint != nil {
		if err := m.exportCheckpoint("after_publish"); err != nil {
			return fmt.Errorf("export published but completion failed: %w", err)
		}
	}
	dir, err := os.Open(filepath.Dir(destination))
	if err != nil {
		return fmt.Errorf("export published but directory sync failed: %w", err)
	}
	err = errors.Join(dir.Sync(), dir.Close())
	if err != nil {
		return fmt.Errorf("export published but directory sync failed: %w", err)
	}
	return nil
}

// Incremental limits include newline bytes, matching Harness's record reader.
// Memory overhead stays at one 64 KiB reader buffer regardless of session size.
func copySessionExport(ctx context.Context, out io.Writer, in io.Reader, maxTotal int64, maxRecord int) error {
	reader := bufio.NewReaderSize(in, 64<<10)
	var total int64
	record := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		part, err := reader.ReadSlice('\n')
		total += int64(len(part))
		record += len(part)
		if total > maxTotal || record > maxRecord {
			return errors.New("session export size limit exceeded")
		}
		if len(part) > 0 {
			n, writeErr := out.Write(part)
			if writeErr != nil {
				return writeErr
			}
			if n != len(part) {
				return io.ErrShortWrite
			}
		}
		if err == nil {
			record = 0
			continue
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
}

// ExportID acquires a saved session writer without changing catalogue selection.
// It requires an existing catalogue identity and does not create a new session.
func (m *Manager) ExportID(ctx context.Context, id, destination string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot, err := m.catalogue.Snapshot()
	if err != nil {
		return err
	}
	for _, record := range snapshot.Sessions {
		if record.ID != id {
			continue
		}
		if _, err := m.SessionPath(record); err != nil {
			return err
		}
		var sess *session.Session
		sess, _, err = m.openDurable(ctx, record.StoreKey)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, sess.Close()) }()
		if sess.ID != id {
			return errors.New("backend session identity differs from catalogue")
		}
		return m.Export(ctx, sess, record.StoreKey, destination)
	}
	return errors.New("session does not belong to this workspace and agent")
}
