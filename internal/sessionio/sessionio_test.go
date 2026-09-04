package sessionio_test

import (
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/sessionio"
)

func TestKeyForWorkspace_StableForSamePath(t *testing.T) {
	a := sessionio.KeyForWorkspace("/Users/sausheong/projects/hand")
	b := sessionio.KeyForWorkspace("/Users/sausheong/projects/hand")
	if a != b {
		t.Fatalf("KeyForWorkspace not stable: %q != %q", a, b)
	}
}

func TestKeyForWorkspace_DifferentForDifferentPaths(t *testing.T) {
	a := sessionio.KeyForWorkspace("/Users/sausheong/projects/hand")
	b := sessionio.KeyForWorkspace("/Users/sausheong/projects/aimp")
	if a == b {
		t.Fatalf("KeyForWorkspace returned the same key for two different paths: %q", a)
	}
}

func TestKeyForWorkspace_ValidSinglePathComponent(t *testing.T) {
	key := sessionio.KeyForWorkspace("/Users/sausheong/projects/hand")
	if key == "" {
		t.Fatal("KeyForWorkspace returned an empty key")
	}
	if strings.ContainsAny(key, `/\`) {
		t.Fatalf("KeyForWorkspace returned a key containing a path separator: %q", key)
	}
	if key == "." || key == ".." {
		t.Fatalf("KeyForWorkspace returned a reserved name: %q", key)
	}
}

func TestStoreDir_ReturnsPathUnderHome(t *testing.T) {
	dir, err := sessionio.StoreDir()
	if err != nil {
		t.Fatalf("StoreDir returned error: %v", err)
	}
	if dir == "" {
		t.Fatal("StoreDir returned an empty path")
	}
	if !strings.HasSuffix(dir, "/.hand/sessions") {
		t.Fatalf("StoreDir = %q, want a path ending in /.hand/sessions", dir)
	}
}
