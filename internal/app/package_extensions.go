package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/hand/internal/packages"
)

type PackageRuntimeApproval struct {
	Package        string                 `json:"package"`
	Review         packages.RuntimeReview `json:"review"`
	ApprovedDigest string                 `json:"approved_digest"`
}

// ReviewPackageExtensions converts an explicit verified installed selection into
// the existing startup review. Only separately approved interpreter --version
// probes execute here; extension entrypoints are never started during review.
func ReviewPackageExtensions(ctx context.Context, store *packages.Store, names []string, workspace, snapshotRoot string, approvals []PackageRuntimeApproval) (ExtensionStartup, error) {
	return reviewPackageExtensions(ctx, store, names, workspace, snapshotRoot, approvals, nil, nil)
}

type PackageContainerRuntimeApproval struct {
	Package        string                          `json:"package"`
	Review         packages.ContainerRuntimeReview `json:"review"`
	ApprovedDigest string                          `json:"approved_digest"`
}

func ReviewContainerPackageExtensions(ctx context.Context, store *packages.Store, names []string, workspace, snapshotRoot string, boundary extensions.ContainerLaunch, approvals []PackageContainerRuntimeApproval) (ExtensionStartup, error) {
	return reviewPackageExtensions(ctx, store, names, workspace, snapshotRoot, nil, &boundary, approvals)
}
func reviewPackageExtensions(ctx context.Context, store *packages.Store, names []string, workspace, snapshotRoot string, approvals []PackageRuntimeApproval, boundary *extensions.ContainerLaunch, containerApprovals []PackageContainerRuntimeApproval) (ExtensionStartup, error) {
	var empty ExtensionStartup
	if store == nil || !filepath.IsAbs(workspace) || !filepath.IsAbs(snapshotRoot) {
		return empty, errors.New("store and absolute workspace/snapshot paths required")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	selected, err := store.ResolveSelected(ctx, names)
	if err != nil {
		return empty, err
	}
	required := map[string]packages.Runtime{}
	extensionNames := map[string]bool{}
	count := 0
	for _, p := range selected.Packages {
		for _, r := range p.Manifest.Runtimes {
			required[p.Name+"/"+r.Name] = r
		}
		for _, e := range p.Manifest.Extensions {
			if extensionNames[e.Name] {
				return empty, errors.New("selected packages declare duplicate extension names")
			}
			extensionNames[e.Name] = true
			count++
		}
	}
	if count == 0 || count > 16 {
		return empty, errors.New("selection must declare between 1 and 16 extensions")
	}
	admitted := map[string]PackageRuntimeApproval{}
	containerAdmitted := map[string]PackageContainerRuntimeApproval{}
	if boundary == nil {
		for _, approval := range approvals {
			key := approval.Package + "/" + approval.Review.Requirement.Name
			requirement, ok := required[key]
			if !ok || requirement != approval.Review.Requirement {
				return empty, errors.New("runtime review differs from selected package requirement")
			}
			if _, exists := admitted[key]; exists {
				return empty, errors.New("duplicate runtime approval")
			}
			digest, err := approval.Review.Digest()
			if err != nil || digest != approval.ApprovedDigest {
				return empty, errors.New("explicit matching runtime review approval required")
			}
			admitted[key] = approval
		}
		if len(admitted) != len(required) {
			return empty, errors.New("selected package runtime reviews and approvals are required")
		}
		// Preserve the explicitly selected package/runtime order for predictable probes.
		for _, p := range selected.Packages {
			for _, r := range p.Manifest.Runtimes {
				approval := admitted[p.Name+"/"+r.Name]
				if _, err = packages.ProbeRuntime(ctx, approval.Review, approval.ApprovedDigest); err != nil {
					return empty, err
				}
			}
		}
	} else {
		for _, approval := range containerApprovals {
			key := approval.Package + "/" + approval.Review.Requirement.Name
			requiredRuntime, ok := required[key]
			if !ok || requiredRuntime != approval.Review.Requirement {
				return empty, errors.New("container runtime review differs from package requirement")
			}
			if _, exists := containerAdmitted[key]; exists {
				return empty, errors.New("duplicate container runtime approval")
			}
			r := approval.Review
			if r.Docker != boundary.Docker || r.Socket != boundary.Socket || r.Image != boundary.Image {
				return empty, errors.New("runtime probe image differs from extension boundary")
			}
			digest, e := r.Digest()
			if e != nil || digest != approval.ApprovedDigest {
				return empty, errors.New("explicit matching container runtime approval required")
			}
			containerAdmitted[key] = approval
		}
		if len(containerAdmitted) != len(required) {
			return empty, errors.New("selected container runtime approvals required")
		}
		for _, p := range selected.Packages {
			for _, r := range p.Manifest.Runtimes {
				approval := containerAdmitted[p.Name+"/"+r.Name]
				if _, err = packages.ProbeContainerRuntime(ctx, approval.Review, approval.ApprovedDigest); err != nil {
					return empty, err
				}
			}
		}
	}
	out := ExtensionStartup{Version: 1, SnapshotRoot: snapshotRoot, Identities: map[string]string{}}
	for _, p := range selected.Packages {
		inventory := map[string]packages.File{}
		for _, f := range p.Manifest.Files {
			inventory[f.Path] = f
		}
		for _, e := range p.Manifest.Extensions {
			cfg := extensions.LaunchConfig{Name: e.Name, Workspace: workspace, PackageDir: p.Directory, Capabilities: append([]string(nil), e.Capabilities...), Arguments: append([]string(nil), e.Arguments...), Order: len(out.Reviews), Mandatory: slices.Contains(e.Capabilities, "policy.check")}
			expectedBinary := inventory[e.Entrypoint]
			if e.Runtime == "" {
				cfg.Executable = filepath.Join(p.Directory, e.Entrypoint)
			} else if boundary != nil {
				approval := containerAdmitted[p.Name+"/"+e.Runtime]
				cfg.Executable = filepath.Join(p.Directory, e.Entrypoint)
				cfg.ImageInterpreter = []string{approval.Review.Executable}
				if approval.Review.Requirement.Name == "python" {
					cfg.ImageInterpreter = append(cfg.ImageInterpreter, "-I", "-B")
				}
			} else {
				approval := admitted[p.Name+"/"+e.Runtime]
				cfg.Executable = approval.Review.Executable
				expectedBinary = packages.File{SHA256: approval.Review.SHA256, Size: approval.Review.Size, Executable: true}
				prefix := []string{}
				if approval.Review.Requirement.Name == "python" {
					prefix = []string{"-I", "-B"}
				}
				cfg.Arguments = append(append(prefix, "${package}/"+e.Entrypoint), cfg.Arguments...)
			}
			for _, f := range p.Manifest.Files {
				// Keep executable resources, including interpreted entrypoints. Only the
				// separately copied native entrypoint may be omitted when no argument uses it.
				if e.Runtime == "" && f.Path == e.Entrypoint && !slices.Contains(cfg.Arguments, "${package}/"+f.Path) {
					continue
				}
				cfg.Files = append(cfg.Files, f.Path)
			}
			var review extensions.LaunchReview
			if boundary != nil {
				review, err = extensions.ReviewContainerLaunch(ctx, cfg, *boundary)
			} else {
				review, err = extensions.ReviewHostLaunch(ctx, cfg)
			}
			if err != nil {
				return empty, fmt.Errorf("package %s extension %s: %w", p.Name, e.Name, err)
			}
			if review.Binary.SHA256 != expectedBinary.SHA256 || review.Binary.Size != expectedBinary.Size {
				return empty, errors.New("extension executable changed after package/runtime verification")
			}
			for _, asset := range review.Assets {
				expected, ok := inventory[asset.Relative]
				if !ok || asset.SHA256 != expected.SHA256 || asset.Size != expected.Size || (asset.Mode&0111 != 0) != expected.Executable {
					return empty, errors.New("package asset changed after verification")
				}
			}
			out.Identities[e.Name] = "package:" + p.Name + ":" + e.Name
			out.Reviews = append(out.Reviews, review)
		}
	}
	return out, nil
}
