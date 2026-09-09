package sessionio

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/sausheong/harness/session"
)

func (m *Manager) forkBoundary(stage string) error {
	if m.forkCheckpoint != nil {
		return m.forkCheckpoint(stage)
	}
	return nil
}

// Fork writes a complete selected history outside the discoverable namespace,
// then atomically links it into that namespace. Reconcile can therefore never
// mistake an interrupted partial copy for a completed fork. Entry IDs are
// scoped to the new session; payload/tool-call references are preserved.
func (m *Manager) Fork(ctx context.Context, history []session.SessionEntry) (Selection, error) {
	var result Selection
	if err := ctx.Err(); err != nil {
		return result, err
	}
	staging, cleanup, err := m.newForkStaging()
	if err != nil {
		return result, err
	}
	defer cleanup()
	store := session.NewStore(staging)
	if err := store.Create(m.agent, "copy"); err != nil {
		return result, err
	}
	copy, err := store.LoadExclusive(m.agent, "copy")
	if err != nil {
		return result, err
	}
	closed := false
	defer func() {
		if !closed {
			copy.Close()
		}
	}()
	if err := BeginUsageTracking(copy); err != nil {
		return result, err
	}
	if err := m.store.CopyAttachments(ctx, m.agent, store, m.agent, history); err != nil {
		return result, err
	}
	for _, entry := range history {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if err := copy.AppendContext(ctx, entry); err != nil {
			return result, err
		}
		if err := m.forkBoundary("copied_entry"); err != nil {
			return result, err
		}
		if err := copy.PersistenceError(); err != nil {
			return result, err
		}
	}
	if err := store.CopyAttachments(ctx, m.agent, m.store, m.agent, copy.Entries()); err != nil {
		return result, err
	}
	if err := copy.Flush(); err != nil {
		return result, err
	}
	if err := copy.Close(); err != nil {
		return result, err
	}
	closed = true
	if err := ctx.Err(); err != nil {
		return result, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return result, err
	}
	key := m.prefix + hex.EncodeToString(random[:])
	targetDir := filepath.Join(m.root, m.agent)
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		return result, err
	}
	if err := m.forkBoundary("before_publish"); err != nil {
		return result, err
	}
	if err := os.Link(filepath.Join(staging, m.agent, "copy.jsonl"), filepath.Join(targetDir, key+".jsonl")); err != nil {
		return result, err
	}
	if err := m.forkBoundary("after_publish"); err != nil {
		return result, err
	}
	// From this point an error leaves a complete recoverable orphan, never a
	// partially copied session. Open validates the newly published graph again.
	next, paths, err := m.openDurable(ctx, key)
	result.Backups = paths
	if err != nil {
		return result, err
	}
	if err := next.Flush(); err != nil {
		return result, errors.Join(err, next.Close())
	}
	record := SessionRecord{ID: next.ID, AgentID: m.agent, StoreKey: key, Name: "Forked session", CreatedAt: time.Now().UTC()}
	if err := ctx.Err(); err != nil {
		return result, errors.Join(err, next.Close())
	}
	if err := m.forkBoundary("before_catalogue"); err != nil {
		return result, errors.Join(err, next.Close())
	}
	if err := m.catalogue.Register(record, true); err != nil {
		return result, errors.Join(err, next.Close())
	}
	result.Session, result.Record = next, record
	return result, nil
}
