package rpc

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
	if f, err := unsupportedLedgerLock(path); err == nil || f != nil {
		t.Fatalf("unsupported operation acquired file authority: %v %v", f, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != string(secret) {
		t.Fatalf("unsupported operation changed sentinel: %q %v", raw, err)
	}
}
