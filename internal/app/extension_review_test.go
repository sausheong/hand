package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExtensionReviewDoesNotExecuteOrOverwrite(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "peer")
	marker := filepath.Join(dir, "executed")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	request := map[string]any{"version": 1, "snapshot_root": filepath.Join(t.TempDir(), "private"), "extensions": []any{map[string]any{"identity": "examples/note", "launch": map[string]any{"name": "note", "executable": exe, "workspace": dir, "capabilities": []string{"commands"}}}}}
	raw, _ := json.Marshal(request)
	input := filepath.Join(dir, "input.json")
	output := filepath.Join(dir, "review.json")
	if err := os.WriteFile(input, raw, 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := WriteExtensionReview(context.Background(), input, output)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := ReadExtensionStartup(output, digest)
	if err != nil || len(approved.Reviews) != 1 {
		t.Fatalf("review: %+v %v", approved, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("review executed peer")
	}
	before, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = WriteExtensionReview(context.Background(), input, output); err == nil {
		t.Fatal("existing review overwritten")
	}
	after, err := os.ReadFile(output)
	if err != nil || string(before) != string(after) {
		t.Fatal("existing review changed")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = WriteExtensionReview(cancelled, input, filepath.Join(dir, "cancelled.json")); err == nil {
		t.Fatal("cancelled review succeeded")
	}
}
