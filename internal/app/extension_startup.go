package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/harness/execution"
)

// ExtensionStartup is explicitly selected by the user, never discovered in a
// project. Approval is the separately supplied SHA-256 of the complete file.
type ExtensionStartup struct {
	Version      int                       `json:"version"`
	SnapshotRoot string                    `json:"snapshot_root"`
	Identities   map[string]string         `json:"identities"`
	Reviews      []extensions.LaunchReview `json:"reviews"`
}

func ReadExtensionStartup(path, approvedDigest string) (ExtensionStartup, error) {
	var selected ExtensionStartup
	if len(approvedDigest) != 64 {
		return selected, errors.New("explicit extension configuration SHA-256 approval required")
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return selected, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return selected, err
	}
	if !info.Mode().IsRegular() || info.Size() > protocol.MaxFrameBytes {
		return selected, errors.New("extension configuration must be a regular file at most 256 KiB")
	}
	raw, err := io.ReadAll(io.LimitReader(f, protocol.MaxFrameBytes+1))
	if err != nil {
		return selected, err
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != approvedDigest {
		return selected, errors.New("extension configuration differs from approved digest")
	}
	if err = protocol.DecodePayload(raw, &selected); err != nil {
		return ExtensionStartup{}, err
	}
	if selected.Version != 1 || len(selected.Reviews) > 16 || len(selected.Identities) != len(selected.Reviews) || !filepath.IsAbs(selected.SnapshotRoot) {
		return ExtensionStartup{}, errors.New("invalid extension startup configuration")
	}
	return selected, nil
}

// ActivateExtensions consumes already reviewed, explicitly approved startup
// metadata. A requested isolation backend must never use this host factory.
func (c *Controller) ActivateExtensions(ctx context.Context, selected ExtensionStartup, workspace string, hostExecution bool) (*ExtensionHost, error) {
	if !hostExecution {
		return nil, errors.New("explicit container backend required; no host fallback")
	}
	return c.activateExtensions(ctx, selected, workspace, nil)
}

// ActivateExtensionsWithBackend requires container reviews to match the selected
// runtime backend exactly. Unknown backend types fail without a host fallback.
func (c *Controller) ActivateExtensionsWithBackend(ctx context.Context, selected ExtensionStartup, workspace string, backend execution.Backend) (*ExtensionHost, error) {
	if backend == nil {
		return c.activateExtensions(ctx, selected, workspace, nil)
	}
	var container execution.Container
	switch b := backend.(type) {
	case execution.Container:
		container = b
	case *execution.Container:
		if b == nil {
			return nil, errors.New("nil container backend")
		}
		container = *b
	default:
		return nil, errors.New("extension execution backend unsupported; no host fallback")
	}
	resolved, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return nil, err
	}
	backendWorkspace, err := filepath.EvalSymlinks(container.Workspace)
	if err != nil || backendWorkspace != resolved {
		return nil, errors.New("extension backend belongs to another workspace")
	}
	boundary := extensions.ContainerLaunch{Docker: container.Docker, Socket: container.Socket, Image: container.Image, Writable: container.Writable, Network: container.Network}
	return c.activateExtensions(ctx, selected, workspace, &boundary)
}

func (c *Controller) activateExtensions(ctx context.Context, selected ExtensionStartup, workspace string, boundary *extensions.ContainerLaunch) (*ExtensionHost, error) {
	operation, release, err := c.owner().reserve(ctx, Running)
	if err != nil {
		return nil, err
	}
	defer release()
	if c.Rt != nil && c.Rt.Compaction.HasInFlight(c.Rt.Session) {
		return nil, errors.New("background compaction is active; retry extension activation after it finishes")
	}
	if c.extensionsActivated {
		return nil, errors.New("extensions already activated; use reviewed reload to reconfigure")
	}
	resolved, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return nil, err
	}
	for _, review := range selected.Reviews {
		if review.Workspace != resolved {
			return nil, errors.New("extension review belongs to another workspace")
		}
		if _, ok := selected.Identities[review.Specification.Name]; !ok {
			return nil, errors.New("extension package identity missing")
		}
	}
	factory, err := reviewedExtensionFactory(selected, boundary)
	if err != nil {
		return nil, err
	}
	host, err := NewExtensionHost(ctx, c, factory, selected.Identities)
	if err != nil {
		return nil, err
	}
	specs := make([]extensions.Specification, len(selected.Reviews))
	for i, r := range selected.Reviews {
		specs[i] = r.Specification
	}
	if _, err = host.manager.Reload(operation, specs); err != nil {
		host.Close()
		return nil, err
	}
	host.workspace = resolved
	host.containerBoundary = boundary
	host.attachRuntimeContext()
	host.attachRuntimePolicy()
	host.attachRuntimeLifecycle()
	host.attachCompactionLifecycle()
	c.extensionsActivated = true
	c.Extensions = host
	host.observeSessionTransition(operation, "", c.SessionID())
	return host, nil
}

func reviewedExtensionFactory(selected ExtensionStartup, boundary *extensions.ContainerLaunch) (extensions.Factory, error) {
	for _, review := range selected.Reviews {
		if boundary == nil {
			if review.Container != nil {
				return nil, errors.New("container review does not match host runtime")
			}
		} else if review.Container == nil || *review.Container != *boundary {
			return nil, errors.New("extension container review differs from selected runtime boundary")
		}
	}
	admit := func(context.Context, extensions.LaunchReview) error { return nil }
	if boundary != nil {
		return extensions.NewContainerFactory(selected.Reviews, admit)
	}
	return extensions.NewHostFactory(selected.SnapshotRoot, selected.Reviews, admit)
}
