package checkpoints

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
	root, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, operation := range []func(*os.File, string, string) error{unsupportedExchangeFiles, unsupportedMoveExclusive} {
		if err := operation(root, "sentinel", "new"); err == nil {
			t.Fatal("unsupported rename accepted")
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "new")); !os.IsNotExist(err) {
		t.Fatalf("unsupported rename created destination: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != string(secret) {
		t.Fatalf("unsupported operation changed sentinel: %q %v", raw, err)
	}
}
