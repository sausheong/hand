package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInteractiveLogsStayOutOfTerminal(t *testing.T) {
	original := slog.Default()
	defer slog.SetDefault(original)
	var terminal bytes.Buffer
	previous := slog.New(slog.NewTextHandler(&terminal, nil))
	slog.SetDefault(previous)
	dir := t.TempDir()
	restore, err := interactiveLogs(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	slog.Warn("tool failed", "error", "exit status 1")
	restore()
	if terminal.Len() != 0 {
		t.Fatal("diagnostic leaked into terminal")
	}
	paths, _ := filepath.Glob(filepath.Join(dir, "logs", "*.log"))
	if len(paths) != 1 {
		t.Fatal(paths)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil || !strings.Contains(string(data), "tool failed") {
		t.Fatalf("%s %v", data, err)
	}
	info, _ := os.Stat(paths[0])
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	if slog.Default() != previous {
		t.Fatal("logger not restored")
	}
}
