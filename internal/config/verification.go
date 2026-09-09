package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const maxVerificationConfigBytes = 64 << 10

type VerificationProfile struct {
	Name    string   `json:"name"`
	Command []string `json:"command"`
}
type VerificationConfig struct {
	MaxRecords int                   `json:"max_records,omitempty"`
	MaxBytes   int64                 `json:"max_bytes,omitempty"`
	Directory  string                `json:"directory"`
	Profiles   []VerificationProfile `json:"profiles"`
}

// ValidatedCopy rejects ambiguous names/argv and isolates caller-owned slices.
// Commands are argv, not implicitly interpreted shell source. Users can select
// a shell explicitly as a command. Loading profiles never executes them.
func (c VerificationConfig) ValidatedCopy() (VerificationConfig, error) {
	if c.Directory == "" || strings.ContainsRune(c.Directory, 0) || !filepath.IsAbs(c.Directory) || len(c.Directory) > 4096 {
		return VerificationConfig{}, errors.New("verification directory must be an absolute path")
	}
	if len(c.Profiles) == 0 || len(c.Profiles) > 32 {
		return VerificationConfig{}, errors.New("verification requires 1-32 profiles")
	}
	if c.MaxRecords == 0 {
		c.MaxRecords = 1000
	}
	if c.MaxBytes == 0 {
		c.MaxBytes = 256 << 20
	}
	if c.MaxRecords < 1 || c.MaxRecords > 10000 || c.MaxBytes < 1 || c.MaxBytes > 8<<30 {
		return VerificationConfig{}, errors.New("invalid verification retention limits")
	}
	result := VerificationConfig{MaxRecords: c.MaxRecords, MaxBytes: c.MaxBytes, Directory: filepath.Clean(c.Directory), Profiles: make([]VerificationProfile, len(c.Profiles))}
	seen := map[string]bool{}
	total := 0
	for i, p := range c.Profiles {
		if len(p.Name) == 0 || len(p.Name) > 64 || strings.ContainsAny(p.Name, " \t\r\n\x00/") || seen[p.Name] {
			return VerificationConfig{}, errors.New("invalid or duplicate verification profile name")
		}
		seen[p.Name] = true
		if len(p.Command) == 0 || len(p.Command) > 256 || p.Command[0] == "" {
			return VerificationConfig{}, errors.New("verification profile requires bounded argv")
		}
		total += len(p.Name)
		for _, arg := range p.Command {
			total += len(arg)
			if len(arg) > 8192 || strings.ContainsRune(arg, 0) {
				return VerificationConfig{}, errors.New("invalid verification argument")
			}
		}
		if total > maxVerificationConfigBytes {
			return VerificationConfig{}, errors.New("verification profiles exceed size limit")
		}
		result.Profiles[i] = VerificationProfile{Name: p.Name, Command: append([]string(nil), p.Command...)}
	}
	return result, nil
}

// ReadVerification reads a bounded, strict JSON configuration without creating
// directories or running commands. Relative evidence paths are resolved against
// the configuration file's directory, never the process working directory.
func ReadVerification(filename string) (VerificationConfig, error) {
	f, err := os.OpenFile(filename, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return VerificationConfig{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return VerificationConfig{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxVerificationConfigBytes {
		return VerificationConfig{}, errors.New("verification config must be a regular file at most 64 KiB")
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxVerificationConfigBytes+1))
	if err != nil {
		return VerificationConfig{}, err
	}
	if len(raw) > maxVerificationConfigBytes {
		return VerificationConfig{}, errors.New("verification config exceeds 64 KiB")
	}
	var c VerificationConfig
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return VerificationConfig{}, fmt.Errorf("verification config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return VerificationConfig{}, errors.New("trailing verification configuration")
	}
	if c.Directory != "" && !filepath.IsAbs(c.Directory) {
		absolute, err := filepath.Abs(filename)
		if err != nil {
			return VerificationConfig{}, err
		}
		c.Directory = filepath.Join(filepath.Dir(absolute), c.Directory)
	}
	return c.ValidatedCopy()
}
