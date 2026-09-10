// Package packages validates explicitly selected Hand distribution packages.
// Reading or validating a manifest never executes its contents or grants trust.
package packages

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/sausheong/hand/extension/protocol"
)

const ManifestName = "hand-package.json"
const MaxFileBytes int64 = 128 << 20
const MaxPackageBytes int64 = 512 << 20

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)
var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)

type Manifest struct {
	Schema        int           `json:"schema"`
	Name          string        `json:"name"`
	Version       string        `json:"version"`
	Compatibility Compatibility `json:"compatibility"`
	Files         []File        `json:"files"`
	Runtimes      []Runtime     `json:"runtimes,omitempty"`
	Extensions    []Extension   `json:"extensions,omitempty"`
}
type Compatibility struct {
	MinimumHand       string `json:"minimum_hand"`
	ExtensionProtocol int    `json:"extension_protocol"`
}
type File struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
	Executable bool   `json:"executable,omitempty"`
}

// Runtime records an external interpreter requirement. Its command is a bare
// executable name, not a shell expression; resolution and approval occur later.
type Runtime struct {
	Name           string `json:"name"`
	Command        string `json:"command"`
	MinimumVersion string `json:"minimum_version"`
}
type Extension struct {
	Name         string   `json:"name"`
	Entrypoint   string   `json:"entrypoint"`
	Runtime      string   `json:"runtime,omitempty"`
	Arguments    []string `json:"arguments,omitempty"`
	Capabilities []string `json:"capabilities"`
}

func validVersion(s string) bool {
	if len(s) > 128 || !versionPattern.MatchString(s) {
		return false
	}
	core := strings.SplitN(s, "+", 2)[0]
	if i := strings.IndexByte(core, '-'); i >= 0 {
		for _, part := range strings.Split(core[i+1:], ".") {
			if len(part) > 1 && part[0] == '0' && strings.Trim(part, "0123456789") == "" {
				return false
			}
		}
	}
	return true
}
func validPath(s string) bool {
	if s == "" || len(s) > 1024 || path.Clean(s) != s || strings.HasPrefix(s, "/") || strings.EqualFold(s, ManifestName) {
		return false
	}
	parts := strings.Split(s, "/")
	if len(parts) > 32 {
		return false
	}
	for _, p := range parts {
		if p == "." || p == ".." || strings.EqualFold(p, ".git") || strings.HasSuffix(p, ".") || strings.HasSuffix(p, " ") {
			return false
		}
		for _, r := range p {
			if r < 33 || r > 126 || strings.ContainsRune(`\:*?"<>|`, r) {
				return false
			}
		}
	}
	return true
}
func (m Manifest) Validate() error {
	if m.Schema != 1 || !namePattern.MatchString(m.Name) || !validVersion(m.Version) || !validVersion(m.Compatibility.MinimumHand) || m.Compatibility.ExtensionProtocol != protocol.Version {
		return errors.New("unsupported package schema, identity, version or compatibility")
	}
	if len(m.Files) == 0 || len(m.Files) > 1024 || len(m.Extensions) > 16 || len(m.Runtimes) > 16 {
		return errors.New("package inventory limits exceeded")
	}
	files := map[string]File{}
	folded := map[string]bool{}
	var total int64
	for _, f := range m.Files {
		key := strings.ToLower(f.Path)
		hash, err := hex.DecodeString(f.SHA256)
		if !validPath(f.Path) || folded[key] || err != nil || len(hash) != 32 || strings.ToLower(f.SHA256) != f.SHA256 || f.Size < 0 || f.Size > MaxFileBytes {
			return fmt.Errorf("invalid or duplicate package file %q", f.Path)
		}
		if !slices.Contains([]string{"skill", "prompt", "extension", "source", "asset"}, f.Kind) || (f.Executable && f.Kind != "extension") {
			return fmt.Errorf("invalid file role %q", f.Path)
		}
		if f.Kind == "skill" && path.Base(f.Path) != "SKILL.md" {
			return errors.New("skill entry must be SKILL.md")
		}
		total += f.Size
		if total > MaxPackageBytes {
			return errors.New("package exceeds 512 MiB")
		}
		files[f.Path] = f
		folded[key] = true
	}
	spellings := map[string]string{}
	for original := range files {
		for part := original; part != "."; part = path.Dir(part) {
			key := strings.ToLower(part)
			if prior, ok := spellings[key]; ok && prior != part {
				return errors.New("package directory case collision")
			}
			spellings[key] = part
		}
	}
	// A path cannot be both a file and a directory, even on a case-folding disk.
	for key := range folded {
		for parent := path.Dir(key); parent != "."; parent = path.Dir(parent) {
			if folded[parent] {
				return errors.New("package file/directory collision")
			}
		}
	}
	runtimes := map[string]bool{}
	for _, r := range m.Runtimes {
		if !namePattern.MatchString(r.Name) || !namePattern.MatchString(r.Command) || !validVersion(r.MinimumVersion) || runtimes[r.Name] {
			return errors.New("invalid or duplicate runtime requirement")
		}
		runtimes[r.Name] = true
	}
	names := map[string]bool{}
	for _, e := range m.Extensions {
		f, ok := files[e.Entrypoint]
		if !namePattern.MatchString(e.Name) || names[e.Name] || !ok || f.Kind != "extension" {
			return errors.New("invalid extension entrypoint or identity")
		}
		if (e.Runtime == "" && !f.Executable) || (e.Runtime != "" && !runtimes[e.Runtime]) {
			return errors.New("extension requires an executable or declared runtime")
		}
		if err := protocol.ValidateCapabilities(e.Capabilities); err != nil {
			return err
		}
		if len(e.Arguments) > 128 {
			return errors.New("too many extension arguments")
		}
		size := 0
		for _, arg := range e.Arguments {
			size += len(arg)
			if !utf8.ValidString(arg) || strings.ContainsRune(arg, 0) {
				return errors.New("NUL in extension argument")
			}
		}
		if size > 32<<10 {
			return errors.New("extension arguments exceed 32 KiB")
		}
		names[e.Name] = true
	}
	raw, err := json.Marshal(m)
	if err != nil || len(raw) > protocol.MaxFrameBytes {
		return errors.New("encoded manifest exceeds 256 KiB")
	}
	return nil
}
func DecodeManifest(raw []byte) (Manifest, error) {
	var m Manifest
	if err := protocol.DecodePayload(raw, &m); err != nil {
		return m, err
	}
	return m, m.Validate()
}

// Digest covers the complete manifest and declared content hashes. Inventory
// order and JSON formatting are irrelevant; command argument order is retained.
func (m Manifest) Digest() (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	var copy Manifest
	if err = json.Unmarshal(raw, &copy); err != nil {
		return "", err
	}
	slices.SortFunc(copy.Files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	slices.SortFunc(copy.Runtimes, func(a, b Runtime) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(copy.Extensions, func(a, b Extension) int { return strings.Compare(a.Name, b.Name) })
	for i := range copy.Extensions {
		slices.Sort(copy.Extensions[i].Capabilities)
	}
	raw, err = json.Marshal(copy)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
