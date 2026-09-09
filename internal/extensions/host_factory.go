package extensions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/sausheong/harness/process"
)

var ErrTransportCleanup = errors.New("extension transport cleanup failed")

type LaunchAdmission func(context.Context, LaunchReview) error

// NewHostFactory supports only explicitly reviewed host execution. A caller
// selecting container isolation must supply a container factory, never this one.
func NewHostFactory(root string, reviews []LaunchReview, admit LaunchAdmission) (Factory, error) {
	if admit == nil || !filepath.IsAbs(root) || len(reviews) > 16 {
		return nil, errors.New("explicit host admission and private snapshot root required")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("snapshot root must be a private nonsymlink directory")
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	byName := map[string]LaunchReview{}
	for _, input := range reviews {
		r := cloneReview(input)
		if r.Container != nil {
			return nil, errors.New("container review cannot use host execution")
		}
		if err := validateLaunchReview(r); err != nil {
			return nil, err
		}
		digest, err := reviewDigest(r)
		if err != nil || digest != r.Specification.Digest {
			return nil, errors.New("launch review digest mismatch")
		}
		normalized, err := normalizeSpecifications([]Specification{r.Specification})
		if err != nil || !reflect.DeepEqual(normalized[0], r.Specification) {
			return nil, errors.New("invalid launch specification")
		}
		if _, exists := byName[r.Specification.Name]; exists {
			return nil, errors.New("duplicate launch review")
		}
		rel, err := filepath.Rel(r.Workspace, root)
		if err != nil || rel == "." || !(rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return nil, errors.New("extension snapshots must be outside the workspace")
		}
		byName[r.Specification.Name] = r
	}
	return func(operation, lifetime context.Context, spec Specification) (*Connection, error) {
		r, ok := byName[spec.Name]
		if !ok || !reflect.DeepEqual(spec, r.Specification) {
			return nil, errors.New("extension launch not reviewed for this specification")
		}
		if err := operation.Err(); err != nil {
			return nil, err
		}
		if err := admit(operation, cloneReview(r)); err != nil {
			return nil, err
		}
		snapshot, err := os.MkdirTemp(root, "peer-")
		if err != nil {
			return nil, err
		}
		retained := false
		defer func() {
			if !retained {
				os.RemoveAll(snapshot)
			}
		}()
		binary := filepath.Join(snapshot, "executable")
		if err = copyReviewed(operation, r.Binary, binary, 0500); err != nil {
			return nil, err
		}
		packageDir := filepath.Join(snapshot, "package")
		for _, asset := range r.Assets {
			mode := os.FileMode(0400)
			if asset.Mode&0111 != 0 {
				mode = 0500
			}
			if err = copyReviewed(operation, asset, filepath.Join(packageDir, filepath.FromSlash(asset.Relative)), mode); err != nil {
				return nil, err
			}
		}
		args := append([]string(nil), r.Arguments...)
		for i, arg := range args {
			if strings.HasPrefix(arg, "${package}/") {
				args[i] = filepath.Join(packageDir, filepath.FromSlash(strings.TrimPrefix(arg, "${package}/")))
			}
		}
		if err = operation.Err(); err != nil {
			return nil, err
		}
		if err = lifetime.Err(); err != nil {
			return nil, err
		}
		owned, cancel := context.WithCancel(lifetime)
		cmd := process.Command(owned, binary, args...)
		cmd.Dir = r.Workspace
		cmd.Env = []string{}
		if _, explicit := r.Environment["PATH"]; !explicit {
			cmd.Env = append(cmd.Env, "PATH=/usr/bin:/bin")
		}
		keys := []string{}
		for key := range r.Environment {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			cmd.Env = append(cmd.Env, key+"="+r.Environment[key])
		}
		input, err := cmd.StdinPipe()
		if err != nil {
			cancel()
			return nil, err
		}
		output, outWriter, err := os.Pipe()
		if err != nil {
			input.Close()
			cancel()
			return nil, err
		}
		diagnostic, errWriter, err := os.Pipe()
		if err != nil {
			input.Close()
			output.Close()
			outWriter.Close()
			cancel()
			return nil, err
		}
		cmd.Stdout = outWriter
		cmd.Stderr = errWriter
		if err = cmd.Start(); err != nil {
			input.Close()
			output.Close()
			outWriter.Close()
			diagnostic.Close()
			errWriter.Close()
			cancel()
			return nil, err
		}
		outWriter.Close()
		errWriter.Close()
		if err = operation.Err(); err != nil {
			cancel()
			process.KillGroup(cmd)
			cmd.Wait()
			input.Close()
			output.Close()
			diagnostic.Close()
			return nil, err
		}
		var stopped atomic.Bool
		transport := Transport{Input: input, Output: output, Stderr: diagnostic, Boundary: r.Boundary,
			Stop: func() { stopped.Store(true); cancel(); process.KillGroup(cmd) },
			Wait: func() error {
				defer cancel()
				runErr := cmd.Wait()
				cleanup := errors.Join(process.KillGroup(cmd), os.RemoveAll(snapshot))
				if stopped.Load() {
					runErr = nil
				}
				if cleanup != nil {
					cleanup = fmt.Errorf("%w: %w", ErrTransportCleanup, cleanup)
				}
				return errors.Join(runErr, cleanup)
			},
		}
		connection, err := Connect(lifetime, transport)
		if err != nil {
			transport.Stop()
			transport.Wait()
			input.Close()
			output.Close()
			diagnostic.Close()
			return nil, err
		}
		retained = true
		return connection, nil
	}, nil
}
func copyReviewed(ctx context.Context, expected LaunchFile, target string, mode os.FileMode) error {
	source, err := os.OpenFile(expected.Path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != expected.Size || uint32(info.Mode().Perm()) != expected.Mode {
		return errors.New("reviewed extension file metadata changed")
	}
	if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(out, hash), io.LimitReader(contextReader{ctx, source}, maxLaunchFileBytes+1))
	if copyErr == nil && (n != expected.Size || hex.EncodeToString(hash.Sum(nil)) != expected.SHA256) {
		copyErr = errors.New("reviewed extension file content changed")
	}
	if copyErr == nil {
		copyErr = out.Chmod(mode)
	}
	if copyErr == nil {
		copyErr = out.Sync()
	}
	return errors.Join(copyErr, out.Close())
}

type contextReader struct {
	ctx    context.Context
	source io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(p)
}
