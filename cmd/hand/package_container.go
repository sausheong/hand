package main

import (
	"context"
	"errors"
	"strings"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/hand/internal/packages"
)

type packageContainerConfig struct {
	Boundary     extensions.ContainerLaunch `json:"boundary"`
	Interpreters map[string]string          `json:"interpreters"`
}
type packageContainerReviews struct {
	Reviews []app.PackageContainerRuntimeApproval `json:"reviews"`
}

func readPackageContainerConfig(ctx context.Context, file string) (packageContainerConfig, error) {
	var cfg packageContainerConfig
	if err := readPackageJSON(file, &cfg); err != nil {
		return cfg, err
	}
	if len(cfg.Interpreters) > 3 {
		return cfg, errors.New("at most three supported image interpreters may be selected")
	}
	// Validate the boundary even for native-only packages without executing it.
	if _, err := packages.ReviewContainerRuntime(ctx, packages.Runtime{Name: "python", Command: "python3", MinimumVersion: "0.0.0"}, cfg.Boundary.Docker, cfg.Boundary.Socket, cfg.Boundary.Image, "/usr/bin/python3"); err != nil {
		return cfg, err
	}
	for name, executable := range cfg.Interpreters {
		if _, err := packages.ReviewContainerRuntime(ctx, packages.Runtime{Name: name, Command: name, MinimumVersion: "0.0.0"}, cfg.Boundary.Docker, cfg.Boundary.Socket, cfg.Boundary.Image, executable); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}
func reviewPackageContainerRuntimes(ctx context.Context, store *packages.Store, names []string, configFile, output string) (any, error) {
	cfg, err := readPackageContainerConfig(ctx, configFile)
	if err != nil {
		return nil, err
	}
	selection, err := store.ResolveSelected(ctx, names)
	if err != nil {
		return nil, err
	}
	reviews := []app.PackageContainerRuntimeApproval{}
	digests := map[string]string{}
	used := map[string]bool{}
	for _, p := range selection.Packages {
		for _, requirement := range p.Manifest.Runtimes {
			executable, ok := cfg.Interpreters[requirement.Name]
			if !ok {
				return nil, errors.New("explicit image interpreter mapping required for " + requirement.Name)
			}
			used[requirement.Name] = true
			review, err := packages.ReviewContainerRuntime(ctx, requirement, cfg.Boundary.Docker, cfg.Boundary.Socket, cfg.Boundary.Image, executable)
			if err != nil {
				return nil, err
			}
			digest, err := review.Digest()
			if err != nil {
				return nil, err
			}
			digests[p.Name+"/"+requirement.Name] = digest
			reviews = append(reviews, app.PackageContainerRuntimeApproval{Package: p.Name, Review: review})
		}
	}
	if len(used) != len(cfg.Interpreters) {
		return nil, errors.New("container interpreter mapping contains unused runtimes")
	}
	filename, err := writePackageReview(output, packageContainerReviews{Reviews: reviews})
	if err != nil {
		return nil, err
	}
	return map[string]any{"review_file": filename, "reviews": reviews, "review_digests": digests}, nil
}
func reviewPackageContainerLaunch(ctx context.Context, store *packages.Store, names []string, workspace, snapshots, configFile, approvalFile string) (app.ExtensionStartup, error) {
	cfg, err := readPackageContainerConfig(ctx, configFile)
	if err != nil {
		return app.ExtensionStartup{}, err
	}
	var approvals packageContainerReviews
	if approvalFile != "" {
		if err = readPackageJSON(approvalFile, &approvals); err != nil {
			return app.ExtensionStartup{}, err
		}
	}
	used := map[string]bool{}
	for _, approval := range approvals.Reviews {
		name := approval.Review.Requirement.Name
		if cfg.Interpreters[name] != approval.Review.Executable || strings.TrimSpace(approval.ApprovedDigest) == "" {
			return app.ExtensionStartup{}, errors.New("approved runtime differs from explicit image interpreter mapping")
		}
		used[name] = true
	}
	if len(used) != len(cfg.Interpreters) {
		return app.ExtensionStartup{}, errors.New("container interpreter mappings require matching approved reviews")
	}
	return app.ReviewContainerPackageExtensions(ctx, store, names, workspace, snapshots, cfg.Boundary, approvals.Reviews)
}
