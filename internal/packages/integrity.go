package packages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"syscall"

	"github.com/sausheong/hand/extension/protocol"
)

// VerifyDirectory checks an explicitly selected package without executing it.
// The caller must revalidate while copying to an owned immutable installation;
// a successful inspection does not freeze a directory another process can edit.
func VerifyDirectory(ctx context.Context, directory string) (Manifest, string, error) {
	var empty Manifest
	if err := ctx.Err(); err != nil {
		return empty, "", err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return empty, "", err
	}
	defer root.Close()
	f, err := openRegular(root, ManifestName)
	if err != nil {
		return empty, "", err
	}
	raw, err := io.ReadAll(io.LimitReader(f, protocol.MaxFrameBytes+1))
	closeErr := f.Close()
	if err != nil {
		return empty, "", err
	}
	if closeErr != nil {
		return empty, "", closeErr
	}
	m, err := DecodeManifest(raw)
	if err != nil {
		return empty, "", err
	}
	inventory := map[string]File{}
	for _, file := range m.Files {
		inventory[file.Path] = file
	}
	seen := map[string]bool{}
	entries := 0
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		entries++
		if entries > 4096 {
			return errors.New("package tree exceeds 4096 entries")
		}
		if p == "." {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("package symlink forbidden: %s", p)
		}
		if d.IsDir() {
			if !validPath(p) {
				return fmt.Errorf("invalid package directory: %s", p)
			}
			return nil
		}
		if p == ManifestName {
			return nil
		}
		expected, ok := inventory[p]
		if !ok {
			return fmt.Errorf("unlisted package file: %s", p)
		}
		file, err := openRegular(root, p)
		if err != nil {
			return err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return err
		}
		if info.Size() != expected.Size || (info.Mode().Perm()&0111 != 0) != expected.Executable {
			return fmt.Errorf("package size or executable mode mismatch: %s", p)
		}
		h := sha256.New()
		buf := make([]byte, 32<<10)
		var count int64
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			n, readErr := file.Read(buf)
			count += int64(n)
			if count > expected.Size {
				return fmt.Errorf("package file grew while reading: %s", p)
			}
			h.Write(buf[:n])
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				return readErr
			}
		}
		if count != expected.Size || hex.EncodeToString(h.Sum(nil)) != expected.SHA256 {
			return fmt.Errorf("package content mismatch: %s", p)
		}
		seen[p] = true
		return nil
	})
	if err != nil {
		return empty, "", err
	}
	if len(seen) != len(inventory) {
		return empty, "", errors.New("declared package file missing")
	}
	digest, err := m.Digest()
	return m, digest, err
}
func openRegular(root *os.Root, path string) (*os.File, error) {
	f, err := root.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		if err == nil {
			err = errors.New("package entry is not a regular file")
		}
		return nil, err
	}
	return f, nil
}
