package packages

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"

	"github.com/sausheong/hand/extension/protocol"
)

var ErrStoreBusy = errors.New("package store already owned")
var ErrStaleReview = errors.New("package store changed since review")

type Revision struct {
	Origin  *Origin `json:"origin,omitempty"`
	Digest  string  `json:"digest"`
	Version string  `json:"version"`
	Source  string  `json:"source"`
}
type Installed struct {
	Name      string     `json:"name"`
	Current   string     `json:"current"`
	Revisions []Revision `json:"revisions"`
}
type Lockfile struct {
	Schema     int         `json:"schema"`
	Generation uint64      `json:"generation"`
	Packages   []Installed `json:"packages"`
}
type ChangeReview struct {
	Origin     *Origin   `json:"origin,omitempty"`
	Action     string    `json:"action"`
	Store      string    `json:"store"`
	Generation uint64    `json:"generation"`
	Name       string    `json:"name"`
	Previous   string    `json:"previous"`
	Digest     string    `json:"digest,omitempty"`
	Source     string    `json:"source,omitempty"`
	Manifest   *Manifest `json:"manifest,omitempty"`
}
type ChangeResult struct {
	Committed  bool   `json:"committed"`
	Generation uint64 `json:"generation"`
}
type Store struct {
	recovery               RecoveryReport
	mu                     sync.Mutex
	directory, handVersion string
	root                   *os.Root
	lock                   *os.File
	closed                 bool
}

func OpenStore(directory, handVersion string) (*Store, error) {
	if handVersion == "" {
		return nil, errors.New("Hand version identity required")
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
		return nil, errors.New("package store must be a private nonsymlink directory (0700)")
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	lock, err := root.OpenFile("store.lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		root.Close()
		return nil, err
	}
	info, err = lock.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		lock.Close()
		root.Close()
		return nil, errors.New("invalid package store lock")
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		root.Close()
		return nil, ErrStoreBusy
	}
	store := &Store{directory: directory, handVersion: handVersion, root: root, lock: lock}
	for _, name := range []string{"objects", "staging"} {
		if err = root.Mkdir(name, 0700); err != nil && !os.IsExist(err) {
			store.Close()
			return nil, err
		}
		info, err = root.Lstat(name)
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
			store.Close()
			return nil, errors.New("invalid package store subdirectory")
		}
	}
	if _, err = store.read(); err != nil {
		store.Close()
		return nil, err
	}
	store.recovery, err = store.recoverTemporaryLocks()
	if err != nil {
		store.Close()
		return nil, err
	}
	return store, nil
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
func validDigest(s string) bool {
	bytes, err := hex.DecodeString(s)
	return err == nil && len(bytes) == 32 && strings.ToLower(s) == s
}
func (s *Store) read() (Lockfile, error) {
	var out Lockfile
	if s.closed {
		return out, errors.New("package store closed")
	}
	f, err := openRegular(s.root, "lock.json")
	if os.IsNotExist(err) {
		return Lockfile{Schema: 1, Packages: []Installed{}}, nil
	}
	if err != nil {
		return out, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, protocol.MaxFrameBytes+1))
	if err != nil {
		return out, err
	}
	if err = protocol.DecodePayload(raw, &out); err != nil {
		return out, err
	}
	if out.Schema != 1 || len(out.Packages) > 128 || out.Generation == 0 {
		return out, errors.New("invalid package lockfile")
	}
	names := map[string]bool{}
	for _, p := range out.Packages {
		if !namePattern.MatchString(p.Name) || names[p.Name] || len(p.Revisions) == 0 || len(p.Revisions) > 32 || !validDigest(p.Current) {
			return out, errors.New("invalid installed package")
		}
		names[p.Name] = true
		digests := map[string]bool{}
		for _, r := range p.Revisions {
			if r.Origin != nil {
				if err := r.Origin.Validate(); err != nil {
					return out, err
				}
			}
			if !validDigest(r.Digest) || !validVersion(r.Version) || len(r.Source) > 2048 || !filepath.IsAbs(r.Source) || digests[r.Digest] {
				return out, errors.New("invalid package revision")
			}
			digests[r.Digest] = true
		}
		if !digests[p.Current] {
			return out, errors.New("current package revision missing")
		}
	}
	return out, nil
}
func (s *Store) List() (Lockfile, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.read() }
func findInstalled(state Lockfile, name string) (Installed, int) {
	for i, p := range state.Packages {
		if p.Name == name {
			return p, i
		}
	}
	return Installed{Name: name}, -1
}
func (r ChangeReview) ApprovalDigest() (string, error) {
	if !filepath.IsAbs(r.Store) || !namePattern.MatchString(r.Name) || (r.Previous != "" && !validDigest(r.Previous)) {
		return "", errors.New("invalid package change review")
	}
	if r.Origin != nil {
		if err := r.Origin.Validate(); err != nil {
			return "", err
		}
	}
	switch r.Action {
	case "install", "rollback":
		if r.Manifest == nil || r.Manifest.Name != r.Name || !filepath.IsAbs(r.Source) || len(r.Source) > 2048 {
			return "", errors.New("invalid package source review")
		}
		digest, err := r.Manifest.Digest()
		if err != nil || digest != r.Digest {
			return "", errors.New("package review digest mismatch")
		}
	case "remove":
		if r.Origin != nil || r.Manifest != nil || r.Digest != "" || r.Source != "" || r.Previous == "" {
			return "", errors.New("invalid removal review")
		}
	default:
		return "", errors.New("unknown package change")
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func (s *Store) PrepareInstall(ctx context.Context, source, pin string) (ChangeReview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.read()
	if err != nil {
		return ChangeReview{}, err
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return ChangeReview{}, err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return ChangeReview{}, err
	}
	m, digest, err := VerifyDirectory(ctx, source)
	if err != nil {
		return ChangeReview{}, err
	}
	if digest != pin {
		return ChangeReview{}, errors.New("package differs from pinned digest")
	}
	if err = m.CheckHandVersion(s.handVersion); err != nil {
		return ChangeReview{}, err
	}
	previous, _ := findInstalled(state, m.Name)
	return ChangeReview{Action: "install", Store: s.directory, Generation: state.Generation, Name: m.Name, Previous: previous.Current, Digest: digest, Source: source, Manifest: &m, Origin: &Origin{Kind: "local", Location: source}}, nil
}
func (s *Store) PrepareRollback(ctx context.Context, name, digest string) (ChangeReview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.read()
	if err != nil {
		return ChangeReview{}, err
	}
	previous, index := findInstalled(state, name)
	if index < 0 {
		return ChangeReview{}, errors.New("package not installed")
	}
	if !slices.ContainsFunc(previous.Revisions, func(r Revision) bool { return r.Digest == digest }) {
		return ChangeReview{}, errors.New("rollback revision not retained")
	}
	source := filepath.Join(s.directory, "objects", digest)
	m, actual, err := VerifyDirectory(ctx, source)
	if err != nil {
		return ChangeReview{}, err
	}
	if actual != digest || m.Name != name {
		return ChangeReview{}, errors.New("retained package integrity mismatch")
	}
	if err = m.CheckHandVersion(s.handVersion); err != nil {
		return ChangeReview{}, err
	}
	var origin *Origin
	for _, revision := range previous.Revisions {
		if revision.Digest == digest && revision.Origin != nil {
			copy := *revision.Origin
			origin = &copy
		}
	}
	return ChangeReview{Action: "rollback", Store: s.directory, Generation: state.Generation, Name: name, Previous: previous.Current, Digest: digest, Source: source, Manifest: &m, Origin: origin}, nil
}
func (s *Store) PrepareRemove(name string) (ChangeReview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.read()
	if err != nil {
		return ChangeReview{}, err
	}
	previous, index := findInstalled(state, name)
	if index < 0 {
		return ChangeReview{}, errors.New("package not installed")
	}
	return ChangeReview{Action: "remove", Store: s.directory, Generation: state.Generation, Name: name, Previous: previous.Current}, nil
}

// Apply requires approval of the full reviewed change and rechecks generation,
// content and compatibility. Installed content is not automatically activated.
func (s *Store) Apply(ctx context.Context, review ChangeReview, approvedDigest string) (result ChangeResult, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	digest, err := review.ApprovalDigest()
	if err != nil {
		return result, err
	}
	if approvedDigest == "" || digest != approvedDigest || review.Store != s.directory {
		return result, errors.New("explicit package change approval required")
	}
	state, err := s.read()
	if err != nil {
		return result, err
	}
	previous, index := findInstalled(state, review.Name)
	if state.Generation != review.Generation || previous.Current != review.Previous {
		return result, ErrStaleReview
	}
	if state.Generation == ^uint64(0) {
		return result, errors.New("package generation exhausted")
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if review.Action == "remove" {
		if index < 0 {
			return result, errors.New("package not installed")
		}
		state.Packages = append(state.Packages[:index], state.Packages[index+1:]...)
	} else {
		if err = review.Manifest.CheckHandVersion(s.handVersion); err != nil {
			return result, err
		}
		retained := slices.ContainsFunc(previous.Revisions, func(r Revision) bool { return r.Digest == review.Digest })
		if review.Action == "rollback" && (!retained || review.Source != filepath.Join(s.directory, "objects", review.Digest)) {
			return result, errors.New("rollback source is not a retained revision")
		}
		if !retained && len(previous.Revisions) >= 32 {
			return result, errors.New("package revision quota reached")
		}
		if index < 0 && len(state.Packages) >= 128 {
			return result, errors.New("installed package quota reached")
		}
		snapshot, e := s.stageReviewedSource(ctx, review)
		if e != nil {
			return result, e
		}
		defer func() { err = errors.Join(err, snapshot.Close()) }()
		object := filepath.Join(s.directory, "objects", review.Digest)
		if _, e = os.Lstat(object); os.IsNotExist(e) {
			if e = os.Rename(snapshot.Directory(), object); e != nil {
				return result, e
			}
			// The snapshot moved to retained content; never remove it on a later lock
			// failure. An unreferenced object is safe and can be collected separately.
			snapshot.closed = true
			d, e := s.root.Open("objects")
			if e != nil {
				return result, e
			}
			e = d.Sync()
			ce := d.Close()
			if e != nil || ce != nil {
				return result, errors.Join(e, ce)
			}
		} else if e != nil {
			return result, e
		} else {
			info, e := os.Lstat(object)
			if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return result, errors.New("invalid retained object")
			}
			_, actual, e := VerifyDirectory(ctx, object)
			if e != nil {
				return result, e
			}
			if actual != review.Digest {
				return result, errors.New("retained object digest mismatch")
			}
		}
		if !retained {
			previous.Revisions = append(previous.Revisions, Revision{Digest: review.Digest, Version: review.Manifest.Version, Source: review.Source, Origin: review.Origin})
		}
		previous.Current = review.Digest
		if index < 0 {
			state.Packages = append(state.Packages, previous)
		} else {
			state.Packages[index] = previous
		}
	}
	state.Generation++
	slices.SortFunc(state.Packages, func(a, b Installed) int { return strings.Compare(a.Name, b.Name) })
	return s.commit(ctx, state)
}
func (s *Store) commit(ctx context.Context, state Lockfile) (result ChangeResult, err error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return result, err
	}
	if len(raw) > protocol.MaxFrameBytes {
		return result, errors.New("package lockfile exceeds 256 KiB")
	}
	name := "lock-" + rand.Text() + ".tmp"
	f, err := s.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return result, err
	}
	defer func() {
		e := s.root.Remove(name)
		if e != nil && !os.IsNotExist(e) {
			err = errors.Join(err, e)
		}
	}()
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return result, err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return result, err
	}
	if err = f.Close(); err != nil {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if err = s.root.Rename(name, "lock.json"); err != nil {
		return result, err
	}
	result = ChangeResult{true, state.Generation}
	d, err := s.root.Open(".")
	if err != nil {
		return result, err
	}
	err = d.Sync()
	return result, errors.Join(err, d.Close())
}

func (s *Store) stageReviewedSource(ctx context.Context, review ChangeReview) (*Snapshot, error) {
	parent := filepath.Join(s.directory, "staging")
	if review.Action == "install" && review.Origin != nil {
		switch review.Origin.Kind {
		case "archive":
			return ImportArchive(ctx, review.Origin.Location, parent, review.Origin.Reference, review.Digest)
		case "git":
			return ImportGit(ctx, review.Origin.Location, parent, review.Origin.Reference, review.Digest)
		}
	}
	return StageDirectory(ctx, review.Source, parent, review.Digest)
}
