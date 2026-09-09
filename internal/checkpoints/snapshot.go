// Package checkpoints captures bounded, non-mutating workspace state. Captures
// must run while Hand's workspace mutations are quiescent; concurrent external
// editors can still prevent a coherent snapshot and are not locked by Hand.
package checkpoints

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"syscall"
)

type Limits struct {
	MaxFiles                    int
	MaxFileBytes, MaxTotalBytes int64
	Exclude                     []string
}

func DefaultLimits() Limits {
	return Limits{MaxFiles: 10000, MaxFileBytes: 8 << 20, MaxTotalBytes: 64 << 20}
}

type Record struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Mode uint32 `json:"mode"`
	Size int64  `json:"size"`
	Kind string `json:"kind"`
}
type Omission struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}
type Snapshot struct {
	records    []Record
	omitted    []Omission
	content    map[string][]byte
	digest     string
	exclusions []string
}

func (s *Snapshot) Digest() string        { return s.digest }
func (s *Snapshot) Records() []Record     { return append([]Record(nil), s.records...) }
func (s *Snapshot) Omissions() []Omission { return append([]Omission(nil), s.omitted...) }

// Content returns an independent copy; callers cannot mutate checkpoint bytes.
func (s *Snapshot) Content(hash string) ([]byte, bool) {
	b, ok := s.content[hash]
	return append([]byte(nil), b...), ok
}

// Generated recovery names are reserved at every directory depth. Keep them
// out of snapshot content while disclosing omissions; recovery owns their bytes.
func recoveryFilename(name string) bool {
	const prefix = ".hand-restore-"
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	suffix := strings.TrimPrefix(name, prefix)
	if len(suffix) != 32 || suffix != strings.ToLower(suffix) {
		return false
	}
	_, err := hex.DecodeString(suffix)
	return err == nil
}

func excluded(name string, extra []string) bool {
	for _, part := range strings.Split(name, "/") {
		if recoveryFilename(part) || part == ".git" || part == ".hand" || part == ".ssh" || part == ".aws" || part == ".env" || strings.HasPrefix(part, ".env.") {
			return true
		}
	}
	for _, p := range extra {
		if name == p || strings.HasPrefix(name, p+"/") {
			return true
		}
	}
	return false
}

// Capture never writes the workspace and does not depend on Git. Size limits
// fail the whole capture rather than silently claiming a partial checkpoint.
// Excluded paths and symbolic links are explicitly listed and never read.
func Capture(ctx context.Context, directory string, limits Limits) (*Snapshot, error) {
	if limits.MaxFiles <= 0 || limits.MaxFiles > 100000 || limits.MaxFileBytes <= 0 || limits.MaxFileBytes > 128<<20 || limits.MaxTotalBytes <= 0 || limits.MaxTotalBytes > 512<<20 {
		return nil, errors.New("invalid checkpoint limits")
	}
	for _, p := range limits.Exclude {
		if !fs.ValidPath(p) || p == "." {
			return nil, fmt.Errorf("invalid exclusion %q", p)
		}
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	s := &Snapshot{content: map[string][]byte{}, exclusions: append([]string(nil), limits.Exclude...)}
	var count int
	var total int64
	err = walkBounded(ctx, root, ".", 0, func(name string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if name == "." {
			return nil
		}
		count++
		if count > limits.MaxFiles {
			return errors.New("checkpoint entry limit exceeded")
		}
		if excluded(name, limits.Exclude) {
			s.omitted = append(s.omitted, Omission{name, "excluded"})
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := root.Lstat(name)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			s.omitted = append(s.omitted, Omission{name, "symbolic link"})
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported checkpoint file type: %s", name)
		}
		if info.Size() > limits.MaxFileBytes || info.Size() > limits.MaxTotalBytes-total {
			return fmt.Errorf("checkpoint byte limit exceeded: %s", name)
		}
		// Nonblocking open prevents a concurrently substituted FIFO from hanging.
		f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		opened, err := f.Stat()
		if err != nil {
			f.Close()
			return err
		}
		if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
			f.Close()
			return fmt.Errorf("checkpoint file changed: %s", name)
		}
		capacity := limits.MaxFileBytes
		if left := limits.MaxTotalBytes - total; left < capacity {
			capacity = left
		}
		b, err := readContext(ctx, io.LimitReader(f, capacity+1))
		after, statErr := f.Stat()
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if statErr != nil {
			return statErr
		}
		if closeErr != nil {
			return closeErr
		}
		final, err := root.Lstat(name)
		if err != nil {
			return err
		}
		if int64(len(b)) > capacity {
			return fmt.Errorf("checkpoint byte limit exceeded: %s", name)
		}
		if !os.SameFile(opened, final) || final.Mode() != opened.Mode() || after.Size() != opened.Size() || final.Size() != opened.Size() || !after.ModTime().Equal(opened.ModTime()) || !final.ModTime().Equal(opened.ModTime()) {
			return fmt.Errorf("checkpoint file changed: %s", name)
		}
		total += int64(len(b))
		h := sha256.Sum256(b)
		hash := hex.EncodeToString(h[:])
		s.content[hash] = b
		s.records = append(s.records, Record{name, hash, uint32(opened.Mode().Perm()), int64(len(b)), "regular"})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(s.records, func(i, j int) bool { return s.records[i].Path < s.records[j].Path })
	sort.Slice(s.omitted, func(i, j int) bool { return s.omitted[i].Path < s.omitted[j].Path })
	// Include omission scope: a result for excluded content is never represented
	// as evidence for the complete workspace.
	payload, err := json.Marshal(struct {
		Records    []Record
		Omissions  []Omission
		Exclusions []string
	}{s.records, s.omitted, s.exclusions})
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256(payload)
	s.digest = hex.EncodeToString(h[:])
	return s, nil
}

func readContext(ctx context.Context, r io.Reader) ([]byte, error) {
	var result []byte
	buf := make([]byte, 32<<10)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := r.Read(buf)
		result = append(result, buf[:n]...)
		if err == io.EOF {
			return result, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

type Change struct {
	Path          string
	Before, After *Record
}

// Changes compares content and executable modes; it never restores files.
func Changes(before, after *Snapshot) []Change {
	old := map[string]Record{}
	next := map[string]Record{}
	names := map[string]bool{}
	for _, r := range before.records {
		old[r.Path] = r
		names[r.Path] = true
	}
	for _, r := range after.records {
		next[r.Path] = r
		names[r.Path] = true
	}
	keys := make([]string, 0, len(names))
	for n := range names {
		keys = append(keys, n)
	}
	sort.Strings(keys)
	var changes []Change
	for _, n := range keys {
		a, okA := old[n]
		b, okB := next[n]
		if okA && okB && a == b {
			continue
		}
		c := Change{Path: path.Clean(n)}
		if okA {
			c.Before = &a
		}
		if okB {
			c.After = &b
		}
		changes = append(changes, c)
	}
	return changes
}

// Read directory entries in fixed batches rather than WalkDir's unbounded
// per-directory allocation. Depth is capped to bound open directory handles.
func walkBounded(ctx context.Context, root *os.Root, name string, depth int, visit fs.WalkDirFunc) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if depth > 64 {
		return errors.New("checkpoint directory depth limit exceeded")
	}
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return err
	}
	if !opened.IsDir() || !os.SameFile(info, opened) {
		return errors.New("checkpoint directory changed")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, readErr := f.ReadDir(128)
		for _, entry := range entries {
			child := path.Join(name, entry.Name())
			err := visit(child, entry, nil)
			if err == fs.SkipDir {
				continue
			}
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if err := walkBounded(ctx, root, child, depth+1, visit); err != nil {
					return err
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	after, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !os.SameFile(opened, after) || !opened.ModTime().Equal(after.ModTime()) {
		return errors.New("checkpoint directory changed")
	}
	return nil
}
