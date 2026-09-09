package main

import (
	"context"
	"encoding/hex"
	"errors"
	"path/filepath"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/packages"
	"github.com/sausheong/harness/runtime"
)

type packageSkillStartup struct {
	Version int                         `json:"version"`
	Store   string                      `json:"store"`
	Skills  []app.PackageSkillSelection `json:"skills"`
}

func readPackageSkillStartup(filename string) (packageSkillStartup, error) {
	var selected packageSkillStartup
	if err := readPackageJSON(filename, &selected); err != nil {
		return selected, err
	}
	if selected.Version != 1 || !filepath.IsAbs(selected.Store) || len(selected.Skills) > 64 {
		return selected, errors.New("package skills require version 1, an absolute store and at most 64 selections")
	}
	for _, skill := range selected.Skills {
		digest, err := hex.DecodeString(skill.Digest)
		if err != nil || len(digest) != 32 || hex.EncodeToString(digest) != skill.Digest {
			return selected, errors.New("each package skill requires a lowercase SHA-256 digest")
		}
	}
	return selected, nil
}
func applyPackageSkillStartup(ctx context.Context, rt *runtime.Runtime, selected packageSkillStartup, version string) (err error) {
	store, err := packages.OpenStore(selected.Store, version)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	controller := &app.Controller{Rt: rt}
	return controller.SelectPackageSkills(ctx, store, selected.Skills)
}
