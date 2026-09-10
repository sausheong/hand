package packages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const MaxTextResourceBytes = 64 << 10

type TextResource struct {
	origin        *ResolvedPackage
	Package       string `json:"package"`
	PackageDigest string `json:"package_digest"`
	Generation    uint64 `json:"generation"`
	Path          string `json:"path"`
	Kind          string `json:"kind"`
	SHA256        string `json:"sha256"`
	Text          string `json:"text"`
	SourcePath    string `json:"source_path"`
	BaseDirectory string `json:"base_directory"`
}

// ReadRelativeResource reads declared text from the already selected package
// revision. It does not consult the current installation or execute resources.
// Every read rechecks the manifest hash; removal retains content-addressed data.
func (r TextResource) ReadRelativeResource(ctx context.Context, relative string) (TextResource, error) {
	if r.origin == nil || relative == "" || strings.HasPrefix(relative, "/") || strings.Contains(relative, "\\") {
		return TextResource{}, errors.New("relative resource requires a selected package and a relative path")
	}
	selected := path.Join(path.Dir(r.Path), relative)
	if !fs.ValidPath(selected) || selected == "." {
		return TextResource{}, errors.New("resource escapes the selected package")
	}
	return readResolvedResource(ctx, *r.origin, r.Generation, selected, true)
}

// ReadTextResource loads an explicitly selected installed skill or prompt. It
// revalidates the package and the returned bytes; loading does not execute code
// or grant the package any additional filesystem or tool authority.
func (s *Store) ReadTextResource(ctx context.Context, name, path string) (TextResource, error) {
	resources, err := s.ReadTextResources(ctx, []TextResourceSelection{{Package: name, Path: path}})
	if err != nil {
		return TextResource{}, err
	}
	return resources[0], nil
}

type TextResourceSelection struct {
	Package string
	Path    string
}

// ReadTextResources resolves every package against one inventory generation and
// then reads immutable content-addressed objects in the caller's selection order.
func (s *Store) ReadTextResources(ctx context.Context, selections []TextResourceSelection) ([]TextResource, error) {
	if len(selections) == 0 || len(selections) > 64 {
		return nil, errors.New("select between 1 and 64 text resources")
	}
	names := []string{}
	seen := map[string]bool{}
	for _, selection := range selections {
		if !seen[selection.Package] {
			names = append(names, selection.Package)
			seen[selection.Package] = true
		}
	}
	selected, err := s.ResolveSelected(ctx, names)
	if err != nil {
		return nil, err
	}
	inventory := map[string]ResolvedPackage{}
	for _, p := range selected.Packages {
		inventory[p.Name] = p
	}
	resources := []TextResource{}
	total := 0
	for _, selection := range selections {
		resource, err := readResolvedText(ctx, inventory[selection.Package], selected.Generation, selection.Path)
		if err != nil {
			return nil, err
		}
		total += len(resource.Text)
		if total > 256<<10 {
			return nil, errors.New("text selection exceeds 256 KiB")
		}
		resources = append(resources, resource)
	}
	return resources, nil
}

func readResolvedText(ctx context.Context, p ResolvedPackage, generation uint64, path string) (TextResource, error) {
	return readResolvedResource(ctx, p, generation, path, false)
}

func readResolvedResource(ctx context.Context, p ResolvedPackage, generation uint64, path string, auxiliary bool) (TextResource, error) {
	var empty TextResource
	var declared *File
	for i := range p.Manifest.Files {
		if p.Manifest.Files[i].Path == path {
			declared = &p.Manifest.Files[i]
			break
		}
	}
	if declared == nil || (!auxiliary && declared.Kind != "skill" && declared.Kind != "prompt") {
		return empty, errors.New("select an inventoried skill or prompt")
	}
	if declared.Size > MaxTextResourceBytes {
		return empty, errors.New("text resource exceeds 64 KiB")
	}
	root, err := os.OpenRoot(p.Directory)
	if err != nil {
		return empty, err
	}
	defer root.Close()
	file, err := openRegular(root, path)
	if err != nil {
		return empty, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, MaxTextResourceBytes+1))
	if err != nil {
		return empty, err
	}
	if err = ctx.Err(); err != nil {
		return empty, err
	}
	sum := sha256.Sum256(raw)
	if int64(len(raw)) != declared.Size || hex.EncodeToString(sum[:]) != declared.SHA256 {
		return empty, errors.New("text resource changed after package verification")
	}
	if !utf8.Valid(raw) {
		return empty, errors.New("text resource must be valid UTF-8")
	}
	source := filepath.Join(p.Directory, filepath.FromSlash(path))
	p.Manifest.Files = append([]File(nil), p.Manifest.Files...)
	return TextResource{Package: p.Name, PackageDigest: p.Digest, Generation: generation,
		Path: path, Kind: declared.Kind, SHA256: declared.SHA256, Text: string(raw),
		SourcePath: source, BaseDirectory: filepath.Dir(source), origin: &p}, nil
}
