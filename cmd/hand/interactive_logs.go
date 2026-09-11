package main

import (
	"log/slog"
	"os"
	"path/filepath"
)

// The full-screen renderer must be the only terminal writer. User-facing
// failures still arrive through application events; raw diagnostics go to disk.
func interactiveLogs(configPath string) (func(), error) {
	dir := filepath.Join(filepath.Dir(configPath), "logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(dir, "interactive-*.log")
	if err != nil {
		return nil, err
	}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(file, &slog.HandlerOptions{Level: slog.LevelWarn})))
	return func() { slog.SetDefault(previous); _ = file.Close() }, nil
}
