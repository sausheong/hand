package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnsupportedPlatformOperationsPreserveWorkspace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sentinel")
	secret := []byte("must remain private and unchanged")
	if err := os.WriteFile(path, secret, 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if f, err := unsupportedLegacySettings(root); err == nil || f != nil {
		t.Fatalf("unsupported legacy inspection acquired access: %v %v", f, err)
	}
	if conn, err := unsupportedStdioRPC(); err == nil || conn != nil {
		t.Fatalf("unsupported RPC acquired a connection: %v %v", conn, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != string(secret) {
		t.Fatalf("unsupported operation changed sentinel: %q %v", raw, err)
	}
}
