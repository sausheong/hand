package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/hand/internal/permissions"
	"io"
	"os"
	"path/filepath"
)

func openCLIAuth(workspace string, cfg config.Config, profile config.ModelProfile) (*permissions.Authority, permissions.LegacyMigration, string, error) {
	canonical, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return nil, permissions.LegacyMigration{}, "", err
	}
	digest, err := cliAuthorityDigest(cfg, profile)
	if err != nil {
		return nil, permissions.LegacyMigration{}, "", err
	}
	workspaceHash := sha256.Sum256([]byte(canonical))
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, permissions.LegacyMigration{}, "", err
	}
	directory := filepath.Join(home, ".hand", "authority", hex.EncodeToString(workspaceHash[:]), digest)
	authority, err := permissions.OpenAuthority(directory, canonical, digest)
	if err != nil {
		return nil, permissions.LegacyMigration{}, "", fmt.Errorf("approval authority %q: %w", directory, err)
	}
	var settings permissions.Settings
	root, err := os.OpenRoot(workspace)
	if err != nil {
		authority.Close()
		return nil, permissions.LegacyMigration{}, "", err
	}
	defer root.Close()
	f, err := openLegacySettings(root)
	if err == nil {
		info, e := f.Stat()
		if e != nil || !info.Mode().IsRegular() {
			f.Close()
			authority.Close()
			return nil, permissions.LegacyMigration{}, "", errors.New("legacy settings must be a regular file")
		}
		data, e := io.ReadAll(io.LimitReader(f, 65537))
		f.Close()
		if e != nil {
			authority.Close()
			return nil, permissions.LegacyMigration{}, "", e
		}
		if len(data) > 65536 {
			authority.Close()
			return nil, permissions.LegacyMigration{}, "", errors.New("legacy settings exceed 64 KiB")
		}
		if e = json.Unmarshal(data, &settings); e != nil {
			authority.Close()
			return nil, permissions.LegacyMigration{}, "", e
		}
	} else if !os.IsNotExist(err) {
		authority.Close()
		return nil, permissions.LegacyMigration{}, "", err
	}
	proposal, err := permissions.PrepareLegacyMigration(settings, canonical, digest)
	if err != nil {
		authority.Close()
		return nil, permissions.LegacyMigration{}, "", err
	}
	return authority, proposal, digest, nil
}

func cliAuthorityDigest(cfg config.Config, profile config.ModelProfile) (string, error) {
	raw, err := json.Marshal(struct {
		Schema  string
		Config  config.Config
		Profile config.ModelProfile
	}{"cli-scoped-v1", cfg, profile})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
