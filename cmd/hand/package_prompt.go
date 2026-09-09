package main

import (
	"context"
	"errors"
	"github.com/sausheong/hand/internal/packages"
	"path/filepath"
)

func readInstalledPackagePrompt(ctx context.Context, filename, version string) (resource packages.TextResource, err error) {
	var selected struct {
		Version int    `json:"version"`
		Store   string `json:"store"`
		Package string `json:"package"`
		Digest  string `json:"digest"`
		Path    string `json:"path"`
	}
	if err = readPackageJSON(filename, &selected); err != nil {
		return resource, err
	}
	if selected.Version != 1 || !filepath.IsAbs(selected.Store) {
		return resource, errors.New("package prompt requires version 1 and an absolute store")
	}
	store, err := packages.OpenStore(selected.Store, version)
	if err != nil {
		return resource, err
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	resource, err = store.ReadTextResource(ctx, selected.Package, selected.Path)
	if err != nil {
		return packages.TextResource{}, err
	}
	if resource.Kind != "prompt" || resource.PackageDigest != selected.Digest {
		return packages.TextResource{}, errors.New("selected resource is not the pinned package prompt")
	}
	return resource, nil
}
