package packages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sausheong/harness/process"
)

// ImportGit exports a pinned commit from an explicitly selected local checkout
// or bare repository using fixed read-only Git commands. It does not check out
// a worktree, invoke hooks/filters or fetch submodules. Remote fetch is separate.
func ImportGit(ctx context.Context, repository, parent, commit, packageDigest string) (snapshot *Snapshot, err error) {
	hash, e := hex.DecodeString(commit)
	if e != nil || len(hash) != 20 || strings.ToLower(commit) != commit || !validDigest(packageDigest) {
		return nil, errors.New("full lowercase Git SHA-1 commit and package SHA-256 pins required")
	}
	info, err := os.Lstat(parent)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
		return nil, errors.New("Git staging parent must be private and nonsymlink (0700)")
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return nil, err
	}
	repository, err = filepath.Abs(repository)
	if err != nil {
		return nil, err
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("Git executable required: %w", err)
	}
	git, err = filepath.Abs(git)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	work, err := os.MkdirTemp(parent, "package-git-")
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, os.RemoveAll(work))
		if err != nil && snapshot != nil {
			err = errors.Join(err, snapshot.Close())
			snapshot = nil
		}
	}()
	run := func(output io.Writer, args ...string) error {
		fixed := []string{"--no-pager", "--no-optional-locks", "-C", repository}
		command := process.Command(ctx, git, append(fixed, args...)...)
		command.Dir = work
		command.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + work, "LANG=C", "LC_ALL=C", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_NO_REPLACE_OBJECTS=1", "GIT_TERMINAL_PROMPT=0"}
		command.Stdout = output
		diagnostics := process.NewCapture(4096)
		command.Stderr = diagnostics
		if err := process.Run(command); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("Git object export failed: %w", err)
		}
		return nil
	}
	capture := process.NewCapture(64)
	if err = run(capture, "cat-file", "-t", commit); err != nil {
		return nil, err
	}
	kind, _, truncated := capture.Snapshot()
	if truncated || strings.TrimSpace(kind) != "commit" {
		return nil, errors.New("Git pin must identify a commit object")
	}
	// Inspect modes before archive export, which otherwise omits submodule data.
	listing := process.NewCapture(8 << 20)
	if err = run(listing, "ls-tree", "-r", "-z", commit); err != nil {
		return nil, err
	}
	records, _, truncated := listing.Snapshot()
	if truncated {
		return nil, errors.New("Git tree inventory exceeds 8 MiB")
	}
	entries := strings.Split(strings.TrimSuffix(records, "\x00"), "\x00")
	if len(entries) > 4096 {
		return nil, errors.New("Git package entry quota exceeded")
	}
	for _, entry := range entries {
		fields := strings.SplitN(entry, "\t", 2)
		if len(fields) != 2 {
			return nil, errors.New("invalid Git tree inventory")
		}
		metadata := strings.Fields(fields[0])
		if len(metadata) != 3 || metadata[1] != "blob" || (metadata[0] != "100644" && metadata[0] != "100755") {
			return nil, errors.New("Git symlinks, submodules and special modes are forbidden")
		}
		if fields[1] != ManifestName && !validPath(fields[1]) {
			return nil, errors.New("unsafe Git package path")
		}
	}
	archiveName := filepath.Join(work, "package.tar")
	archive, err := os.OpenFile(archiveName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	checksum := sha256.New()
	limited := &gitArchiveWriter{writer: io.MultiWriter(archive, checksum), remaining: MaxPackageBytes, cancel: cancel}
	err = run(limited, "archive", "--format=tar", commit)
	closeErr := archive.Close()
	if err != nil || closeErr != nil {
		return nil, errors.Join(err, closeErr)
	}
	snapshot, err = ImportArchive(ctx, archiveName, parent, hex.EncodeToString(checksum.Sum(nil)), packageDigest)
	if err == nil {
		snapshot.origin = Origin{Kind: "git", Location: repository, Reference: commit}
	}
	return snapshot, err
}

type gitArchiveWriter struct {
	writer    io.Writer
	remaining int64
	cancel    context.CancelFunc
}

func (w *gitArchiveWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > w.remaining {
		w.cancel()
		return 0, errors.New("Git archive exceeds 512 MiB")
	}
	n, err := w.writer.Write(data)
	w.remaining -= int64(n)
	return n, err
}
