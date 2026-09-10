//go:build darwin || linux

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/config"
)

func TestLegacySettingsRejectionReleasesAuthority(t *testing.T) {
	for _, name := range []string{"directory", "fifo", "symlink", "oversized", "malformed", "invalid-tool"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			workspace := t.TempDir()
			dir := filepath.Join(workspace, ".hand")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "settings.json")
			var raw []byte
			var err error
			switch name {
			case "directory":
				err = os.Mkdir(path, 0700)
			case "fifo":
				err = syscall.Mkfifo(path, 0600)
			case "symlink":
				target := filepath.Join(t.TempDir(), "settings.json")
				raw = []byte(`{"always_allow":["bash"]}`)
				if err = os.WriteFile(target, raw, 0600); err != nil {
					t.Fatal(err)
				}
				err = os.Symlink(target, path)
			case "oversized":
				raw = []byte(strings.Repeat(" ", 65537))
				err = os.WriteFile(path, raw, 0600)
			case "malformed":
				raw = []byte(`{"always_allow":`)
				err = os.WriteFile(path, raw, 0600)
			case "invalid-tool":
				raw = []byte(`{"always_allow":[" bash"]}`)
				err = os.WriteFile(path, raw, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			cfg := config.Config{}
			profile := config.ModelProfile{Provider: "local", Model: "fixture"}
			start := time.Now()
			a, _, _, err := openCLIAuth(workspace, cfg, profile)
			if a != nil {
				a.Close()
			}
			if err == nil || a != nil {
				t.Fatal("unsafe settings accepted")
			}
			if time.Since(start) > time.Second {
				t.Fatal("nonregular settings blocked startup")
			}
			after, err := os.Lstat(path)
			if err != nil || before.Mode() != after.Mode() {
				t.Fatal("settings type/mode changed", err)
			}
			if raw != nil {
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(raw, got) {
					t.Fatal("rejection changed settings", err)
				}
			}
			if err = os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, []byte(`{"always_allow":["bash"]}`), 0600); err != nil {
				t.Fatal(err)
			}
			a, proposal, _, err := openCLIAuth(workspace, cfg, profile)
			if err != nil {
				t.Fatal("failed startup leaked authority lock", err)
			}
			defer a.Close()
			if len(a.Grants()) != 0 || len(proposal.Grants()) != 1 {
				t.Fatal("recovery self-authorised proposal")
			}
		})
	}
}
