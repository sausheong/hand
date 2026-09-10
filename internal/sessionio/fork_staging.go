package sessionio

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxForkStagingScan = 256
const maxForkStagingCleanup = 32

// The root lock closes the create-before-owner-lock race. Each active copy
// retains its own lock; cleanup never infers liveness from a timestamp or PID.
func (m *Manager) newForkStaging() (string, func(), error) {
	rootLock, err := lockCatalogue(filepath.Join(m.root, ".session-staging.lock"))
	if err != nil {
		return "", nil, err
	}
	defer rootLock.Close()
	dir, err := os.MkdirTemp(m.root, ".fork-")
	if err != nil {
		return "", nil, err
	}
	owner, err := lockCatalogue(filepath.Join(dir, ".owner.lock"))
	if err != nil {
		os.RemoveAll(dir)
		return "", nil, err
	}
	return dir, func() { os.RemoveAll(dir); owner.Close() }, nil
}

// CleanupForkStaging is bounded by directory-entry and removal counts. A busy
// root lock means another creator/cleaner owns this pass, so it is deferred.
func (m *Manager) CleanupForkStaging(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	rootLock, err := lockCatalogue(filepath.Join(m.root, ".session-staging.lock"))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if errors.Is(err, ErrCatalogueBusy) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer rootLock.Close()
	root, err := os.Open(m.root)
	if err != nil {
		return 0, err
	}
	defer root.Close()
	entries, err := root.ReadDir(maxForkStagingScan)
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		if removed == maxForkStagingCleanup {
			break
		}
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ".fork-") {
			continue
		}
		dir := filepath.Join(m.root, entry.Name())
		owner, err := lockCatalogue(filepath.Join(dir, ".owner.lock"))
		if errors.Is(err, ErrCatalogueBusy) {
			continue
		}
		if err != nil {
			return removed, err
		}
		err = os.RemoveAll(dir)
		closeErr := owner.Close()
		if err := errors.Join(err, closeErr); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}
