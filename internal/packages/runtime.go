package packages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

const runtimeBoundary = "host execution of reviewed interpreter --version; not an OS sandbox; runtime libraries are not snapshotted"

type RuntimeReview struct {
	Requirement Runtime `json:"requirement"`
	Executable  string  `json:"executable"`
	SHA256      string  `json:"sha256"`
	Size        int64   `json:"size"`
	Boundary    string  `json:"boundary"`
}

func validateRuntime(r Runtime) error {
	if !namePattern.MatchString(r.Name) || !namePattern.MatchString(r.Command) || !validVersion(r.MinimumVersion) {
		return errors.New("invalid runtime requirement")
	}
	switch r.Name {
	case "python", "node", "ruby":
		return nil
	default:
		return fmt.Errorf("runtime %s needs a host version-probe adapter", r.Name)
	}
}

// ReviewRuntime resolves and fingerprints an interpreter without executing it.
// PATH lookup is discovery only: the resulting review needs separate approval.
func ReviewRuntime(ctx context.Context, requirement Runtime) (RuntimeReview, error) {
	var out RuntimeReview
	if err := validateRuntime(requirement); err != nil {
		return out, err
	}
	selected, err := exec.LookPath(requirement.Command)
	if err != nil {
		return out, fmt.Errorf("runtime %s missing (%s): %w", requirement.Name, requirement.Command, err)
	}
	selected, err = filepath.Abs(selected)
	if err != nil {
		return out, err
	}
	selected, err = filepath.EvalSymlinks(selected)
	if err != nil {
		return out, err
	}
	root, err := os.OpenRoot(filepath.Dir(selected))
	if err != nil {
		return out, err
	}
	defer root.Close()
	f, err := openRegular(root, filepath.Base(selected))
	if err != nil {
		return out, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return out, err
	}
	if info.Size() > MaxFileBytes || info.Mode().Perm()&0111 == 0 {
		return out, errors.New("runtime must be an executable regular file at most 128 MiB")
	}
	hash := sha256.New()
	buffer := make([]byte, 32<<10)
	var size int64
	for {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		n, e := f.Read(buffer)
		size += int64(n)
		if size > MaxFileBytes {
			return out, errors.New("runtime exceeds size limit")
		}
		hash.Write(buffer[:n])
		if e == io.EOF {
			break
		}
		if e != nil {
			return out, e
		}
	}
	if size != info.Size() {
		return out, errors.New("runtime changed during review")
	}
	return RuntimeReview{requirement, selected, hex.EncodeToString(hash.Sum(nil)), size, runtimeBoundary}, nil
}
func (r RuntimeReview) Digest() (string, error) {
	if err := validateRuntime(r.Requirement); err != nil {
		return "", err
	}
	hash, err := hex.DecodeString(r.SHA256)
	if !filepath.IsAbs(r.Executable) || err != nil || len(hash) != 32 || strings.ToLower(r.SHA256) != r.SHA256 || r.Size < 0 || r.Size > MaxFileBytes || r.Boundary != runtimeBoundary {
		return "", errors.New("invalid runtime review")
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// ProbeRuntime executes only a separately approved review, from a private copy
// revalidated against its fingerprint. It never loads package extension source.
func ProbeRuntime(ctx context.Context, review RuntimeReview, approvedDigest string) (version string, err error) {
	digest, err := review.Digest()
	if err != nil {
		return "", err
	}
	if approvedDigest == "" || digest != approvedDigest {
		return "", errors.New("explicit runtime review approval required")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err = ctx.Err(); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "hand-runtime-probe-")
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	if err = os.Chmod(dir, 0700); err != nil {
		return "", err
	}
	input, err := os.OpenRoot(filepath.Dir(review.Executable))
	if err != nil {
		return "", err
	}
	defer input.Close()
	output, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer output.Close()
	base := filepath.Base(review.Executable)
	if err = copyPackageFile(ctx, input, output, File{Path: base, SHA256: review.SHA256, Size: review.Size, Executable: true}); err != nil {
		return "", fmt.Errorf("prepare reviewed runtime snapshot: %w", err)
	}
	command := process.Command(ctx, filepath.Join(dir, base), "--version")
	command.Dir = dir
	command.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + dir, "LANG=C", "LC_ALL=C"}
	stdout, stderr := process.NewCapture(4096), process.NewCapture(4096)
	command.Stdout = stdout
	command.Stderr = stderr
	if err = process.Run(command); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("execute reviewed runtime version probe: %w", ctx.Err())
		}
		return "", fmt.Errorf("runtime version probe failed: %w", err)
	}
	outText, _, outTruncated := stdout.Snapshot()
	errText, _, errTruncated := stderr.Snapshot()
	if outTruncated || errTruncated {
		return "", errors.New("runtime version response exceeds 4 KiB per stream")
	}
	version, err = parseRuntimeVersion(review.Requirement.Name, outText+"\n"+errText)
	if err != nil {
		return "", err
	}
	comparison, err := CompareVersions(version, review.Requirement.MinimumVersion)
	if err != nil {
		return "", err
	}
	if comparison < 0 {
		return version, fmt.Errorf("runtime %s requires >= %s; found %s", review.Requirement.Name, review.Requirement.MinimumVersion, version)
	}
	return version, nil
}
func parseRuntimeVersion(name, raw string) (string, error) {
	fields := strings.Fields(strings.TrimSpace(raw))
	var version string
	switch name {
	case "python":
		if len(fields) == 2 && fields[0] == "Python" {
			version = fields[1]
		}
	case "node":
		if len(fields) == 1 {
			version = strings.TrimPrefix(fields[0], "v")
		}
	case "ruby":
		if len(fields) >= 2 && fields[0] == "ruby" {
			version = strings.SplitN(fields[1], "p", 2)[0]
		}
	}
	if !validVersion(version) {
		return "", fmt.Errorf("unrecognised %s version response", name)
	}
	return version, nil
}
