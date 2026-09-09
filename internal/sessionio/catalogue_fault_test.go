package sessionio

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestCatalogueFailuresBeforeRenamePreserveOldIndex(t *testing.T) {
	for _, stage := range []string{"file_sync", "rename"} {
		t.Run(stage, func(t *testing.T) {
			c := catalogueFixture(t)
			if err := c.Register(catalogueRecord("one"), true); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(c.dir, "catalogue.json")
			before, _ := os.ReadFile(path)
			ops := c.operations()
			if stage == "file_sync" {
				ops.syncFile = func(*os.File) error { return syscall.ENOSPC }
			} else {
				ops.rename = func(string, string) error { return syscall.EACCES }
			}
			c.io = &ops
			if err := c.Register(catalogueRecord("two"), true); err == nil {
				t.Fatal("storage failure hidden")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("failed update destroyed previous index")
			}
			temps, _ := filepath.Glob(filepath.Join(c.dir, ".catalogue-*"))
			if len(temps) != 0 {
				t.Fatal("failed update leaked staging file")
			}
			c.io = nil
			if err := c.Register(catalogueRecord("two"), true); err != nil {
				t.Fatal("failed update retained lock", err)
			}
		})
	}
}

func TestCatalogueIdempotentRetryReestablishesDurability(t *testing.T) {
	c := catalogueFixture(t)
	if err := c.Register(catalogueRecord("one"), true); err != nil {
		t.Fatal(err)
	}
	ops := c.operations()
	failure := errors.New("directory sync failed")
	ops.syncDirectory = func(string) error { return failure }
	c.io = &ops
	if err := c.Register(catalogueRecord("two"), false); !errors.Is(err, failure) {
		t.Fatal("post-rename failure hidden", err)
	}
	snapshot, err := c.Snapshot()
	if err != nil || len(snapshot.Sessions) != 2 {
		t.Fatal("post-rename result not complete", err)
	}
	path := filepath.Join(c.dir, "catalogue.json")
	before, _ := os.ReadFile(path)
	if err := c.Register(catalogueRecord("two"), false); !errors.Is(err, failure) {
		t.Fatal("idempotent retry falsely reported durable success", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("idempotent retry rewrote metadata")
	}
	c.io = nil
	if err := c.Register(catalogueRecord("two"), false); err != nil {
		t.Fatal("durability retry failed after fault cleared", err)
	}
	snapshot, err = c.Snapshot()
	if err != nil || snapshot.Generation != 2 {
		t.Fatal("retry duplicated registration", err)
	}
}

func TestCatalogueIdempotentRetryReportsFileSyncFailure(t *testing.T) {
	c := catalogueFixture(t)
	if err := c.Register(catalogueRecord("one"), false); err != nil {
		t.Fatal(err)
	}
	ops := c.operations()
	ops.syncFile = func(*os.File) error { return syscall.EIO }
	c.io = &ops
	if err := c.Register(catalogueRecord("one"), false); !errors.Is(err, syscall.EIO) {
		t.Fatal("idempotent file sync error hidden", err)
	}
}

func TestCatalogueBoundsAndGenerationExhaustion(t *testing.T) {
	c := catalogueFixture(t)
	if err := os.MkdirAll(c.dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(c.dir, "catalogue.json")
	if err := os.WriteFile(path, []byte(strings.Repeat(" ", MaxCatalogueBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Snapshot(); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatal("byte cap not enforced", err)
	}
	tooMany := CatalogueSnapshot{Version: 1, Workspace: c.workspace, Sessions: make([]SessionRecord, MaxCatalogueSessions+1)}
	raw, err := json.Marshal(tooMany)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Snapshot(); err == nil || !strings.Contains(err.Error(), "session limit") {
		t.Fatal("session cap not enforced", err)
	}
	exhausted := CatalogueSnapshot{Version: 1, Workspace: c.workspace, Generation: ^uint64(0), Sessions: []SessionRecord{catalogueRecord("one")}}
	raw, err = json.Marshal(exhausted)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.Select("one"); err == nil || !strings.Contains(err.Error(), "generation exhausted") {
		t.Fatal("generation wrapped", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(raw, after) {
		t.Fatal("exhausted generation changed index")
	}
}
