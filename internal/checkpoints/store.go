package checkpoints

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
)

const maxEncodedSnapshot = 192 << 20

type StoreLimits struct {
	MaxSnapshots int
	MaxBytes     int64
}

func DefaultStoreLimits() StoreLimits { return StoreLimits{100, 512 << 20} }

// Store owns an exclusive process lock. Retention never silently deletes a
// checkpoint: capacity exhaustion requires an explicit Delete operation.
type Store struct {
	workspace string
	syncFile  func(*os.File) error
	syncDir   func() error
	publish   func(string, string) error // publication seam also used by fault tests
	mu        sync.Mutex
	root      *os.Root
	lock      *os.File
	limits    StoreLimits
	closed    bool
	failed    error
}
type storedSnapshot struct {
	Version    int
	Records    []Record
	Omissions  []Omission
	Exclusions []string
	Content    map[string][]byte
	Digest     string
}

func OpenStore(directory, workspace string, limits StoreLimits) (*Store, error) {
	if limits.MaxSnapshots <= 0 || limits.MaxSnapshots > 10000 || limits.MaxBytes <= 0 || limits.MaxBytes > 8<<30 {
		return nil, errors.New("invalid checkpoint store limits")
	}
	work, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return nil, err
	}
	work, err = filepath.Abs(work)
	if err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	// Resolve existing ancestors before creating anything to reject workspace
	// aliases even when the final store directory does not yet exist.
	ancestor := absolute
	var tail []string
	for {
		_, err = os.Lstat(ancestor)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		tail = append(tail, filepath.Base(ancestor))
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return nil, err
		}
		ancestor = parent
	}
	resolved, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return nil, err
	}
	for i := len(tail) - 1; i >= 0; i-- {
		resolved = filepath.Join(resolved, tail[i])
	}
	rel, err := filepath.Rel(work, resolved)
	if err != nil {
		return nil, err
	}
	if rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, errors.New("checkpoint store must be outside workspace")
	}
	if err = os.MkdirAll(resolved, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("checkpoint store must be a private directory")
	}
	root, err := os.OpenRoot(resolved)
	if err != nil {
		return nil, err
	}
	lock, err := root.OpenFile(".lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		root.Close()
		return nil, err
	}
	li, err := lock.Stat()
	if err != nil || !li.Mode().IsRegular() || li.Mode().Perm()&0077 != 0 {
		lock.Close()
		root.Close()
		return nil, errors.New("invalid checkpoint lock")
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		root.Close()
		return nil, fmt.Errorf("checkpoint store already owned: %w", err)
	}
	s := &Store{workspace: work, root: root, lock: lock, limits: limits, publish: root.Rename, syncFile: (*os.File).Sync}
	s.syncDir = func() error {
		f, err := root.Open(".")
		if err != nil {
			return err
		}
		return errors.Join(f.Sync(), f.Close())
	}
	if err = s.recoverPending(); err != nil {
		s.Close()
		return nil, err
	}
	if _, _, err = s.inventory(); err != nil {
		s.Close()
		return nil, err
	}
	if _, err = s.readRestoreJournal(context.Background()); err != nil {
		s.Close()
		return nil, err
	}
	if err = s.syncDirectory(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) ready() error {
	if s.closed {
		return errors.New("checkpoint store closed")
	}
	return s.failed
}
func validDigest(id string) bool {
	b, err := hex.DecodeString(id)
	return err == nil && len(b) == 32 && id == strings.ToLower(id)
}
func (s *Store) inventory() ([]string, int64, error) {
	f, err := s.root.Open(".")
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	var ids []string
	var total int64
	count := 0
	for {
		entries, e := f.ReadDir(128)
		for _, d := range entries {
			if d.Name() == ".lock" {
				continue
			}
			if d.Name() != restoreJournalName {
				count++
			}
			if count > s.limits.MaxSnapshots {
				return nil, 0, errors.New("checkpoint store entry limit exceeded")
			}
			info, err := s.root.Lstat(d.Name())
			if err != nil {
				return nil, 0, err
			}
			if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
				return nil, 0, errors.New("invalid checkpoint store entry")
			}
			total += info.Size()
			if total > s.limits.MaxBytes {
				return nil, 0, errors.New("checkpoint store byte limit exceeded")
			}
			if d.Name() == restoreJournalName {
				continue
			}
			if strings.HasPrefix(d.Name(), ".pending-") {
				return nil, 0, errors.New("interrupted checkpoint write requires recovery")
			}
			if !strings.HasSuffix(d.Name(), ".json") || !validDigest(strings.TrimSuffix(d.Name(), ".json")) {
				return nil, 0, errors.New("unknown checkpoint store entry")
			}
			ids = append(ids, strings.TrimSuffix(d.Name(), ".json"))
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, 0, e
		}
	}
	sort.Strings(ids)
	return ids, total, nil
}
func (s *Store) List() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ready(); err != nil {
		return nil, err
	}
	ids, _, err := s.inventory()
	return ids, err
}
func (s *Store) Load(ctx context.Context, id string) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ready(); err != nil {
		return nil, err
	}
	return s.load(ctx, id)
}
func (s *Store) load(ctx context.Context, id string) (*Snapshot, error) {
	if !validDigest(id) {
		return nil, errors.New("invalid checkpoint id")
	}
	f, err := s.root.OpenFile(id+".json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > maxEncodedSnapshot {
		return nil, errors.New("invalid checkpoint file")
	}
	b, err := readContext(ctx, io.LimitReader(f, maxEncodedSnapshot+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxEncodedSnapshot {
		return nil, errors.New("checkpoint encoding limit exceeded")
	}
	var disk storedSnapshot
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&disk); err != nil {
		return nil, err
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return nil, errors.New("trailing checkpoint data")
	}
	if disk.Version != 1 || disk.Digest != id {
		return nil, errors.New("checkpoint identity mismatch")
	}
	snap := &Snapshot{records: disk.Records, omitted: disk.Omissions, exclusions: disk.Exclusions, content: disk.Content, digest: disk.Digest}
	if err = validateSnapshot(snap); err != nil {
		return nil, err
	}
	return snap, nil
}
func validateSnapshot(s *Snapshot) error {
	if s == nil || !validDigest(s.digest) || len(s.records)+len(s.omitted) > 100000 {
		return errors.New("invalid checkpoint")
	}
	seen := map[string]bool{}
	used := map[string]bool{}
	last := ""
	for _, r := range s.records {
		if !fs.ValidPath(r.Path) || r.Path == "." || r.Path <= last || excluded(r.Path, s.exclusions) || r.Kind != "regular" || r.Mode > 0777 || !validDigest(r.Hash) {
			return errors.New("invalid checkpoint record")
		}
		last = r.Path
		seen[r.Path] = true
		b, ok := s.content[r.Hash]
		if !ok || int64(len(b)) != r.Size {
			return errors.New("checkpoint content size mismatch")
		}
		h := sha256.Sum256(b)
		if hex.EncodeToString(h[:]) != r.Hash {
			return errors.New("checkpoint content hash mismatch")
		}
		used[r.Hash] = true
	}
	if len(used) != len(s.content) {
		return errors.New("unreferenced checkpoint content")
	}
	last = ""
	for _, o := range s.omitted {
		if !fs.ValidPath(o.Path) || o.Path == "." || o.Path <= last || seen[o.Path] || (o.Reason != "excluded" && o.Reason != "symbolic link") {
			return errors.New("invalid checkpoint omission")
		}
		last = o.Path
	}
	for _, p := range s.exclusions {
		if !fs.ValidPath(p) || p == "." {
			return errors.New("invalid checkpoint exclusion")
		}
	}
	data, err := json.Marshal(struct {
		Records    []Record
		Omissions  []Omission
		Exclusions []string
	}{s.records, s.omitted, s.exclusions})
	if err != nil {
		return err
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != s.digest {
		return errors.New("checkpoint digest mismatch")
	}
	return nil
}
func (s *Store) Save(ctx context.Context, snap *Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ready(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateSnapshot(snap); err != nil {
		return err
	}
	ids, total, err := s.inventory()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if id == snap.digest {
			_, err := s.load(ctx, id)
			return err
		}
	}
	if len(ids) >= s.limits.MaxSnapshots {
		return errors.New("checkpoint retention capacity reached")
	}
	b, err := json.Marshal(storedSnapshot{1, snap.records, snap.omitted, snap.exclusions, snap.content, snap.digest})
	if err != nil {
		return err
	}
	if len(b) > maxEncodedSnapshot || int64(len(b)) > s.limits.MaxBytes-total {
		return errors.New("checkpoint storage byte limit exceeded")
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return err
	}
	temp := ".pending-" + hex.EncodeToString(nonce[:])
	f, err := s.root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer s.root.Remove(temp)
	for len(b) > 0 {
		if err = ctx.Err(); err != nil {
			f.Close()
			return err
		}
		n := len(b)
		if n > 32<<10 {
			n = 32 << 10
		}
		written, e := f.Write(b[:n])
		if e != nil {
			f.Close()
			return e
		}
		if written != n {
			f.Close()
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	if err = s.syncFile(f); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = s.publish(temp, snap.digest+".json"); err != nil {
		return err
	}
	return s.syncDirectory()
}
func (s *Store) syncDirectory() error {
	err := s.syncDir()
	if err != nil {
		s.failed = fmt.Errorf("checkpoint durability uncertain: %w", err)
		return s.failed
	}
	return nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ready(); err != nil {
		return err
	}
	if !validDigest(id) {
		return errors.New("invalid checkpoint id")
	}
	if err := s.root.Remove(id + ".json"); err != nil {
		return err
	}
	return s.syncDirectory()
}
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return errors.Join(s.lock.Close(), s.root.Close())
}

// A pending file was never published under its digest, so removing it cannot
// discard an accepted checkpoint. Run only after obtaining the process lock.
func (s *Store) recoverPending() error {
	f, err := s.root.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	var pending []string
	count := 0
	for {
		entries, e := f.ReadDir(128)
		for _, d := range entries {
			count++
			if count > s.limits.MaxSnapshots+129 {
				return errors.New("checkpoint recovery entry limit exceeded")
			}
			if !strings.HasPrefix(d.Name(), ".pending-") {
				continue
			}
			suffix := strings.TrimPrefix(d.Name(), ".pending-")
			decoded, err := hex.DecodeString(suffix)
			if err != nil || len(decoded) != 16 || suffix != strings.ToLower(suffix) {
				return errors.New("unrecognised pending checkpoint")
			}
			info, err := s.root.Lstat(d.Name())
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
				return errors.New("unsafe pending checkpoint")
			}
			pending = append(pending, d.Name())
			if len(pending) > 128 {
				return errors.New("checkpoint recovery pending limit exceeded")
			}
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
	}
	for _, name := range pending {
		if err = s.root.Remove(name); err != nil {
			return err
		}
	}
	if len(pending) > 0 {
		return s.syncDirectory()
	}
	return nil
}
