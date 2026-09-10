package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInvocationExtensionReviewWithoutProvider(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "peer")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 99\n"), 0700); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, "input.json")
	output := filepath.Join(dir, "review.json")
	raw, _ := json.Marshal(map[string]any{"version": 1, "snapshot_root": filepath.Join(t.TempDir(), "private"), "extensions": []any{map[string]any{"identity": "examples/note", "launch": map[string]any{"name": "note", "executable": exe, "workspace": dir, "capabilities": []string{"commands"}}}}})
	if err := os.WriteFile(input, raw, 0600); err != nil {
		t.Fatal(err)
	}
	invocationFixture(t, "--review-extensions", input, "--extension-review-output", output)
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatal(err)
	}
}

func TestInvocationContainerExtensionReviewWithoutDaemon(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "peer")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 99\n"), 0700); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, "input.json")
	output := filepath.Join(dir, "review.json")
	raw, _ := json.Marshal(map[string]any{"version": 1, "container": map[string]any{"docker": "/unavailable/docker", "socket": "/unavailable/socket", "image": "sha256:" + strings.Repeat("a", 64), "network": false, "writable": false}, "snapshot_root": filepath.Join(t.TempDir(), "private"), "extensions": []any{map[string]any{"identity": "examples/note", "launch": map[string]any{"name": "note", "executable": exe, "workspace": dir, "capabilities": []string{"commands"}}}}})
	if err := os.WriteFile(input, raw, 0600); err != nil {
		t.Fatal(err)
	}
	invocationFixture(t, "--review-extensions", input, "--extension-review-output", output)
	if err := run(); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var reviewed struct {
		Reviews []struct {
			Container map[string]any `json:"container"`
			Boundary  string         `json:"boundary"`
		} `json:"reviews"`
	}
	if err = json.Unmarshal(body, &reviewed); err != nil || len(reviewed.Reviews) != 1 || reviewed.Reviews[0].Container["image"] != "sha256:"+strings.Repeat("a", 64) || !strings.Contains(reviewed.Reviews[0].Boundary, "network=false") {
		t.Fatal(string(body), err)
	}
}
