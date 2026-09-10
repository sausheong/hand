package extensions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

const maxLaunchFileBytes = 128 << 20
const maxLaunchAssetsBytes = 32 << 20

type LaunchConfig struct {
	ImageInterpreter []string          `json:"image_interpreter,omitempty"`
	Name             string            `json:"name"`
	Executable       string            `json:"executable"`
	PackageDir       string            `json:"package_dir,omitempty"`
	Files            []string          `json:"files,omitempty"`
	Arguments        []string          `json:"arguments,omitempty"`
	Workspace        string            `json:"workspace"`
	Environment      map[string]string `json:"environment,omitempty"`
	Capabilities     []string          `json:"capabilities"`
	Order            int               `json:"order,omitempty"`
	Mandatory        bool              `json:"mandatory,omitempty"`
}

type LaunchFile struct {
	Path     string `json:"path"`
	Relative string `json:"relative,omitempty"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	Mode     uint32 `json:"mode"`
}
type LaunchReview struct {
	ImageInterpreter []string          `json:"image_interpreter,omitempty"`
	Container        *ContainerLaunch  `json:"container,omitempty"`
	Specification    Specification     `json:"specification"`
	Binary           LaunchFile        `json:"binary"`
	Assets           []LaunchFile      `json:"assets"`
	Arguments        []string          `json:"arguments"`
	Workspace        string            `json:"workspace"`
	Environment      map[string]string `json:"environment"`
	Boundary         string            `json:"boundary"`
}

func fingerprint(path, relative string) (LaunchFile, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return LaunchFile{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return LaunchFile{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxLaunchFileBytes {
		return LaunchFile{}, errors.New("launch file must be regular and at most 128 MiB")
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(f, maxLaunchFileBytes+1))
	if err != nil {
		return LaunchFile{}, err
	}
	if n != info.Size() || n > maxLaunchFileBytes {
		return LaunchFile{}, errors.New("launch file changed size during review")
	}
	return LaunchFile{Path: path, Relative: relative, SHA256: hex.EncodeToString(hash.Sum(nil)), Size: n, Mode: uint32(info.Mode().Perm())}, nil
}
func reviewDigest(r LaunchReview) (string, error) {
	r.Specification.Digest = ""
	raw, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func cloneReview(r LaunchReview) LaunchReview {
	raw, _ := json.Marshal(r)
	var out LaunchReview
	json.Unmarshal(raw, &out)
	return out
}
func ReviewHostLaunch(ctx context.Context, cfg LaunchConfig) (LaunchReview, error) {
	if len(cfg.ImageInterpreter) > 0 {
		return LaunchReview{}, errors.New("image interpreter requires container launch")
	}
	return reviewLaunchFiles(ctx, cfg)
}
func reviewLaunchFiles(ctx context.Context, cfg LaunchConfig) (LaunchReview, error) {
	var r LaunchReview
	if err := ctx.Err(); err != nil {
		return r, err
	}
	if !filepath.IsAbs(cfg.Executable) || !filepath.IsAbs(cfg.Workspace) || len(cfg.Files) > 64 || len(cfg.Arguments) > 128 || len(cfg.Environment) > 64 {
		return r, errors.New("invalid launch paths or limits")
	}
	exe, err := filepath.EvalSymlinks(cfg.Executable)
	if err != nil {
		return r, err
	}
	workspace, err := filepath.EvalSymlinks(cfg.Workspace)
	if err != nil {
		return r, err
	}
	info, err := os.Stat(workspace)
	if err != nil || !info.IsDir() {
		return r, errors.New("launch workspace unavailable")
	}
	r.Binary, err = fingerprint(exe, "")
	if err != nil {
		return r, err
	}
	if r.Binary.Mode&0111 == 0 && len(cfg.ImageInterpreter) == 0 {
		return r, errors.New("extension executable is not executable")
	}
	r.ImageInterpreter = append([]string(nil), cfg.ImageInterpreter...)
	if err = validateImageInterpreter(r.ImageInterpreter); err != nil {
		return r, err
	}
	r.Arguments = append([]string(nil), cfg.Arguments...)
	r.Workspace = workspace
	r.Environment = map[string]string{}
	size := 0
	for _, arg := range r.Arguments {
		size += len(arg)
		if strings.ContainsRune(arg, 0) {
			return r, errors.New("NUL in extension argument")
		}
		if strings.HasPrefix(arg, "${package}/") && !fs.ValidPath(strings.TrimPrefix(arg, "${package}/")) {
			return r, errors.New("invalid package argument")
		}
	}
	for k, v := range cfg.Environment {
		if k == "" || strings.IndexFunc(k, func(r rune) bool {
			return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_')
		}) >= 0 || strings.ContainsRune(v, 0) {
			return r, errors.New("invalid explicit extension environment")
		}
		size += len(k) + len(v)
		r.Environment[k] = v
	}
	if size > 32<<10 {
		return r, errors.New("extension arguments/environment exceed 32 KiB")
	}
	seen := map[string]bool{}
	var total int64
	if len(cfg.Files) > 0 {
		if !filepath.IsAbs(cfg.PackageDir) {
			return r, errors.New("package directory must be absolute")
		}
		packageDir, err := filepath.EvalSymlinks(cfg.PackageDir)
		if err != nil {
			return r, err
		}
		for _, rel := range cfg.Files {
			if err = ctx.Err(); err != nil {
				return r, err
			}
			if !fs.ValidPath(rel) || rel == "." || strings.Contains(rel, "\\") || seen[rel] {
				return r, errors.New("invalid or duplicate package resource")
			}
			seen[rel] = true
			path := filepath.Join(packageDir, filepath.FromSlash(rel))
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				return r, err
			}
			within, err := filepath.Rel(packageDir, resolved)
			if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
				return r, errors.New("package resource escapes package directory")
			}
			file, err := fingerprint(resolved, rel)
			if err != nil {
				return r, err
			}
			total += file.Size
			if total > maxLaunchAssetsBytes {
				return r, errors.New("package resources exceed 32 MiB")
			}
			r.Assets = append(r.Assets, file)
		}
	}
	for _, arg := range r.Arguments {
		if strings.HasPrefix(arg, "${package}/") && !seen[strings.TrimPrefix(arg, "${package}/")] {
			return r, errors.New("package argument is not a reviewed resource")
		}
	}
	sort.Slice(r.Assets, func(i, j int) bool { return r.Assets[i].Relative < r.Assets[j].Relative })
	r.Boundary = "unrestricted host subprocess; capabilities are not an OS sandbox; runtime libraries are not snapshotted"
	specs, err := normalizeSpecifications([]Specification{{Name: cfg.Name, Digest: strings.Repeat("0", 64), Capabilities: cfg.Capabilities, Order: cfg.Order, Mandatory: cfg.Mandatory}})
	if err != nil {
		return r, err
	}
	r.Specification = specs[0]
	r.Specification.Digest, err = reviewDigest(r)
	return r, err
}

// Reviews may be loaded from disk. A matching digest does not make their paths
// safe: validate the complete snapshot layout again before creating any files.
func validateLaunchReview(r LaunchReview) error {
	if err := validateImageInterpreter(r.ImageInterpreter); err != nil {
		return err
	}
	if len(r.ImageInterpreter) > 0 && r.Container == nil {
		return errors.New("image interpreter without container boundary")
	}
	if r.Container != nil {
		if err := validateContainerLaunch(*r.Container); err != nil {
			return err
		}
	}
	if !filepath.IsAbs(r.Workspace) || len(r.Assets) > 64 || len(r.Arguments) > 128 || len(r.Environment) > 64 || r.Boundary != launchBoundary(r.Container) {
		return errors.New("invalid launch review")
	}
	validFile := func(f LaunchFile) bool {
		hash, err := hex.DecodeString(f.SHA256)
		return filepath.IsAbs(f.Path) && f.Size >= 0 && f.Size <= maxLaunchFileBytes && f.Mode <= 0777 && err == nil && len(hash) == 32 && hex.EncodeToString(hash) == f.SHA256
	}
	if !validFile(r.Binary) || r.Binary.Relative != "" || (r.Binary.Mode&0111 == 0 && len(r.ImageInterpreter) == 0) {
		return errors.New("invalid reviewed executable")
	}
	seen := map[string]bool{}
	var total int64
	for _, f := range r.Assets {
		if !validFile(f) || !fs.ValidPath(f.Relative) || f.Relative == "." || strings.Contains(f.Relative, "\\") || seen[f.Relative] {
			return errors.New("invalid reviewed resource")
		}
		seen[f.Relative] = true
		total += f.Size
	}
	if total > maxLaunchAssetsBytes {
		return errors.New("reviewed resources exceed limit")
	}
	for rel := range seen {
		for parent := filepath.ToSlash(filepath.Dir(rel)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			if seen[parent] {
				return errors.New("resource path conflicts with directory")
			}
		}
	}
	size := 0
	for _, arg := range r.Arguments {
		size += len(arg)
		if strings.ContainsRune(arg, 0) || (strings.HasPrefix(arg, "${package}/") && !seen[strings.TrimPrefix(arg, "${package}/")]) {
			return errors.New("invalid reviewed argument")
		}
	}
	for k, v := range r.Environment {
		if k == "" || strings.IndexFunc(k, func(r rune) bool {
			return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_')
		}) >= 0 || strings.ContainsRune(v, 0) {
			return errors.New("invalid reviewed environment")
		}
		size += len(k) + len(v)
	}
	if size > 32<<10 {
		return errors.New("reviewed arguments/environment exceed limit")
	}
	return nil
}
