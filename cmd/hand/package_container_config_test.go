package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageContainerConfigValidationNeverExecutesDocker(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "invoked")
	t.Setenv("HAND_REVIEW_TEST_MARKER", marker)
	docker := filepath.Join(dir, "docker")
	if err := os.WriteFile(docker, []byte("#!/bin/sh\ntouch \"$HAND_REVIEW_TEST_MARKER\"\nexit 71\n"), 0700); err != nil {
		t.Fatal(err)
	}
	base := map[string]any{"boundary": map[string]any{"docker": docker, "socket": filepath.Join(dir, "socket"), "image": "sha256:" + strings.Repeat("a", 64)}, "interpreters": map[string]string{"python": "/usr/bin/python3"}}
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
		valid  bool
	}{
		{"valid", func(map[string]any) {}, true},
		{"native only", func(m map[string]any) { delete(m, "interpreters") }, true},
		{"mutable image", func(m map[string]any) { m["boundary"].(map[string]any)["image"] = "python:latest" }, false},
		{"relative docker", func(m map[string]any) { m["boundary"].(map[string]any)["docker"] = "docker" }, false},
		{"relative socket", func(m map[string]any) { m["boundary"].(map[string]any)["socket"] = "socket" }, false},
		{"workspace interpreter", func(m map[string]any) { m["interpreters"] = map[string]string{"python": "/workspace/python"} }, false},
		{"noncanonical interpreter", func(m map[string]any) { m["interpreters"] = map[string]string{"python": "/usr/bin/../python"} }, false},
		{"unknown runtime", func(m map[string]any) { m["interpreters"] = map[string]string{"java": "/usr/bin/java"} }, false},
		{"too many runtimes", func(m map[string]any) {
			m["interpreters"] = map[string]string{"python": "/bin/python", "node": "/bin/node", "ruby": "/bin/ruby", "java": "/bin/java"}
		}, false},
		{"unknown field", func(m map[string]any) { m["execute"] = true }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(base)
			var cfg map[string]any
			if err := json.Unmarshal(raw, &cfg); err != nil {
				t.Fatal(err)
			}
			tc.change(cfg)
			raw, err := json.Marshal(cfg)
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(t.TempDir(), "config.json")
			if err = os.WriteFile(file, raw, 0600); err != nil {
				t.Fatal(err)
			}
			got, err := readPackageContainerConfig(context.Background(), file)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%t config=%+v err=%v", tc.valid, got, err)
			}
			if _, err = os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("configuration review invoked Docker", err)
			}
			if tc.valid {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if _, err = readPackageContainerConfig(ctx, file); err != context.Canceled {
					t.Fatalf("cancelled review: %v", err)
				}
			}
		})
	}
}
