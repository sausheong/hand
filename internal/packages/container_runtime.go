package packages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/sausheong/harness/execution"
)

const containerRuntimeBoundary = "version probe in pinned image; network disabled; private empty read-only workspace; no package source loaded"

// ContainerRuntimeReview binds the interpreter path to an immutable image,
// rather than fingerprinting an unrelated host interpreter. Creating a review
// never starts Docker or probes the image.
type ContainerRuntimeReview struct {
	Requirement Runtime `json:"requirement"`
	Docker      string  `json:"docker"`
	Socket      string  `json:"socket"`
	Image       string  `json:"image"`
	Executable  string  `json:"executable"`
	Boundary    string  `json:"boundary"`
}

func ReviewContainerRuntime(ctx context.Context, requirement Runtime, docker, socket, image, executable string) (ContainerRuntimeReview, error) {
	r := ContainerRuntimeReview{requirement, docker, socket, image, executable, containerRuntimeBoundary}
	if err := ctx.Err(); err != nil {
		return ContainerRuntimeReview{}, err
	}
	if _, err := r.Digest(); err != nil {
		return ContainerRuntimeReview{}, err
	}
	return r, nil
}
func (r ContainerRuntimeReview) Digest() (string, error) {
	if err := validateRuntime(r.Requirement); err != nil {
		return "", err
	}
	digest, err := hex.DecodeString(strings.TrimPrefix(r.Image, "sha256:"))
	if !strings.HasPrefix(r.Image, "sha256:") || err != nil || len(digest) != 32 || "sha256:"+hex.EncodeToString(digest) != r.Image || !filepath.IsAbs(r.Docker) || !filepath.IsAbs(r.Socket) || r.Boundary != containerRuntimeBoundary {
		return "", errors.New("invalid container runtime review")
	}
	if !path.IsAbs(r.Executable) || path.Clean(r.Executable) != r.Executable || len(r.Executable) > 4096 || strings.ContainsAny(r.Executable, "\x00\r\n\\") {
		return "", errors.New("invalid image interpreter path")
	}
	for _, prefix := range []string{"/workspace", "/tmp", "/harness-resources", "/hand-worker"} {
		if r.Executable == prefix || strings.HasPrefix(r.Executable, prefix+"/") {
			return "", errors.New("interpreter must reside in immutable image filesystem")
		}
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func ProbeContainerRuntime(ctx context.Context, r ContainerRuntimeReview, approved string) (version string, err error) {
	digest, err := r.Digest()
	if err != nil {
		return "", err
	}
	if approved == "" || approved != digest {
		return "", errors.New("explicit container runtime review approval required")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err = ctx.Err(); err != nil {
		return "", err
	}
	workspace, err := os.MkdirTemp("", "hand-container-runtime-")
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(workspace)) }()
	backend := execution.Container{Docker: r.Docker, Socket: r.Socket, Image: r.Image, Workspace: workspace}
	result, err := backend.Run(ctx, execution.Request{Argv: []string{r.Executable, "--version"}, CaptureLimit: 4096, Env: map[string]string{"LANG": "C", "LC_ALL": "C", "HOME": "/tmp"}})
	if err != nil {
		return "", fmt.Errorf("container runtime probe failed: %w", err)
	}
	if result.ExitCode != 0 || result.StdoutTruncated || result.StderrTruncated {
		return "", errors.New("container runtime probe failed or exceeded 4 KiB per stream")
	}
	version, err = parseRuntimeVersion(r.Requirement.Name, result.Stdout+"\n"+result.Stderr)
	if err != nil {
		return "", err
	}
	comparison, err := CompareVersions(version, r.Requirement.MinimumVersion)
	if err != nil {
		return "", err
	}
	if comparison < 0 {
		return version, fmt.Errorf("runtime %s requires >= %s; found %s", r.Requirement.Name, r.Requirement.MinimumVersion, version)
	}
	return version, nil
}
