package packages

import (
	"context"
	"errors"
	"path/filepath"
)

type ResolvedPackage struct {
	Name      string   `json:"name"`
	Digest    string   `json:"digest"`
	Directory string   `json:"directory"`
	Manifest  Manifest `json:"manifest"`
}
type ResolvedSelection struct {
	Generation uint64            `json:"generation"`
	Packages   []ResolvedPackage `json:"packages"`
}

// ResolveSelected verifies a bounded explicit selection under one store
// generation. Results are data, not execution grants; launch preparation must
// still fingerprint its selected files and obtain separate execution approval.
func (s *Store) ResolveSelected(ctx context.Context, names []string) (ResolvedSelection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out ResolvedSelection
	if len(names) == 0 || len(names) > 16 {
		return out, errors.New("select between 1 and 16 installed packages")
	}
	seen := map[string]bool{}
	for _, name := range names {
		if !namePattern.MatchString(name) || seen[name] {
			return out, errors.New("invalid or duplicate installed package selection")
		}
		seen[name] = true
	}
	state, err := s.read()
	if err != nil {
		return out, err
	}
	out.Generation = state.Generation
	for _, name := range names {
		if err = ctx.Err(); err != nil {
			return ResolvedSelection{}, err
		}
		installed, index := findInstalled(state, name)
		if index < 0 {
			return ResolvedSelection{}, errors.New("selected package not installed: " + name)
		}
		relative := filepath.Join("objects", installed.Current)
		info, err := s.root.Lstat(relative)
		if err != nil {
			return ResolvedSelection{}, err
		}
		if !info.IsDir() {
			return ResolvedSelection{}, errors.New("installed package object must be a nonsymlink directory")
		}
		directory := filepath.Join(s.directory, relative)
		manifest, digest, err := VerifyDirectory(ctx, directory)
		if err != nil {
			return ResolvedSelection{}, err
		}
		if digest != installed.Current || manifest.Name != name {
			return ResolvedSelection{}, errors.New("installed package identity mismatch")
		}
		for _, revision := range installed.Revisions {
			if revision.Digest == digest && revision.Version != manifest.Version {
				return ResolvedSelection{}, errors.New("installed package version metadata mismatch")
			}
		}
		if err = manifest.CheckHandVersion(s.handVersion); err != nil {
			return ResolvedSelection{}, err
		}
		out.Packages = append(out.Packages, ResolvedPackage{name, digest, directory, manifest})
	}
	return out, nil
}
