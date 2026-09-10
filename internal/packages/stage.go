package packages

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
	"path/filepath"
	"strings"
	"sync"
)

// Snapshot owns a private, verified copy. It is not installed or authorised to
// execute. Close removes only this staging directory, never the source package.
type Snapshot struct {
	origin    Origin
	directory string
	digest    string
	mu        sync.Mutex
	closed    bool
}

func (s *Snapshot) Directory() string { return s.directory }
func (s *Snapshot) Digest() string    { return s.digest }
func (s *Snapshot) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	if err := os.RemoveAll(s.directory); err != nil {
		return err
	}
	s.closed = true
	return nil
}

// StageDirectory requires a pinned identity and an existing private staging
// parent outside the source. It never runs package hooks or runtime commands.
func StageDirectory(ctx context.Context, source, parent, expectedDigest string) (snapshot *Snapshot, err error) {
	hash, err := hex.DecodeString(expectedDigest)
	if err != nil || len(hash) != 32 || strings.ToLower(expectedDigest) != expectedDigest {
		return nil, errors.New("pinned package SHA-256 required")
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return nil, err
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return nil, err
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(parent)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
		return nil, errors.New("package staging parent must be a private nonsymlink directory (0700)")
	}
	parent, err = filepath.EvalSymlinks(parent)
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(source, parent)
	if err != nil {
		return nil, err
	}
	if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return nil, errors.New("package staging parent must be outside source")
	}
	m, digest, err := VerifyDirectory(ctx, source)
	if err != nil {
		return nil, err
	}
	if digest != expectedDigest {
		return nil, errors.New("package differs from pinned digest")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	directory, err := os.MkdirTemp(parent, "package-stage-")
	if err != nil {
		return nil, err
	}
	owned := &Snapshot{directory: directory, digest: digest, origin: Origin{Kind: "local", Location: source}}
	defer func() {
		if snapshot == nil {
			err = errors.Join(err, owned.Close())
		}
	}()
	input, err := os.OpenRoot(source)
	if err != nil {
		return nil, err
	}
	defer input.Close()
	output, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer output.Close()
	for _, file := range m.Files {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if err = output.MkdirAll(path.Dir(file.Path), 0700); err != nil {
			return nil, err
		}
		if err = copyPackageFile(ctx, input, output, file); err != nil {
			return nil, err
		}
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	if err = writeSnapshotManifest(output, raw); err != nil {
		return nil, err
	}
	_, copiedDigest, err := VerifyDirectory(ctx, directory)
	if err != nil {
		return nil, err
	}
	if copiedDigest != expectedDigest {
		return nil, errors.New("copied package differs from pinned digest")
	}
	// Persist each containing directory after its files, then the parent entry.
	var directories []string
	err = fs.WalkDir(output.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			directories = append(directories, p)
		}
		return ctx.Err()
	})
	if err != nil {
		return nil, err
	}
	for i := len(directories) - 1; i >= 0; i-- {
		dir, e := output.Open(directories[i])
		if e != nil {
			return nil, e
		}
		e = dir.Sync()
		closeErr := dir.Close()
		if e != nil || closeErr != nil {
			return nil, errors.Join(e, closeErr)
		}
	}
	parentDir, err := os.Open(parent)
	if err != nil {
		return nil, err
	}
	err = parentDir.Sync()
	closeErr := parentDir.Close()
	if err != nil || closeErr != nil {
		return nil, errors.Join(err, closeErr)
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return owned, nil
}
func copyPackageFile(ctx context.Context, input, output *os.Root, expected File) (err error) {
	src, err := openRegular(input, expected.Path)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	if info.Size() != expected.Size || (info.Mode().Perm()&0111 != 0) != expected.Executable {
		return fmt.Errorf("package source metadata changed: %s", expected.Path)
	}
	dst, err := output.OpenFile(expected.Path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, dst.Close()) }()
	hash := sha256.New()
	buffer := make([]byte, 32<<10)
	var total int64
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		n, readErr := src.Read(buffer)
		total += int64(n)
		if total > expected.Size {
			return fmt.Errorf("package source grew: %s", expected.Path)
		}
		if n > 0 {
			if _, err = dst.Write(buffer[:n]); err != nil {
				return err
			}
			hash.Write(buffer[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if total != expected.Size || hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
		return fmt.Errorf("package source content changed: %s", expected.Path)
	}
	mode := os.FileMode(0400)
	if expected.Executable {
		mode = 0500
	}
	if err = dst.Chmod(mode); err != nil {
		return err
	}
	return dst.Sync()
}
func writeSnapshotManifest(output *os.Root, raw []byte) (err error) {
	f, err := output.OpenFile(ManifestName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	if _, err = f.Write(raw); err != nil {
		return err
	}
	if err = f.Chmod(0400); err != nil {
		return err
	}
	return f.Sync()
}
