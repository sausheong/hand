package sessionio

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func catalogueFixture(t *testing.T) *Catalogue {
	t.Helper()
	c, err := OpenCatalogue(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func catalogueRecord(id string) SessionRecord {
	return SessionRecord{ID: id, AgentID: "hand", StoreKey: "key_" + id, Name: "Session " + id, CreatedAt: time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)}
}

func TestCataloguePreservesSessionsAcrossRegisterSelectRenameAndRestart(t *testing.T) {
	c := catalogueFixture(t)
	if err := c.Register(catalogueRecord("one"), true); err != nil {
		t.Fatal(err)
	}
	if err := c.Register(catalogueRecord("two"), true); err != nil {
		t.Fatal(err)
	}
	if err := c.Rename("one", "Original work"); err != nil {
		t.Fatal(err)
	}
	if err := c.Select("one"); err != nil {
		t.Fatal(err)
	}
	reopened := &Catalogue{dir: c.dir, workspace: c.workspace}
	s, err := reopened.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Sessions) != 2 || s.LastActiveID != "one" || s.Sessions[0].Name != "Original work" || s.Sessions[0].LastActive.IsZero() || s.Generation != 4 {
		t.Fatal("catalogue lost session or selection", s)
	}
	raw, err := os.ReadFile(filepath.Join(c.dir, "catalogue.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Register(catalogueRecord("one"), false); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(filepath.Join(c.dir, "catalogue.json"))
	if !bytes.Equal(raw, again) {
		t.Fatal("duplicate registration changed metadata")
	}
	info, err := os.Stat(filepath.Join(c.dir, "catalogue.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("catalogue not private", err)
	}
	s.Sessions[0].Name = "mutated copy"
	actual, _ := c.Snapshot()
	if actual.Sessions[0].Name != "Original work" {
		t.Fatal("snapshot aliases durable data")
	}
}

func TestCatalogueCanonicalWorkspaceAlias(t *testing.T) {
	root := t.TempDir()
	workspace := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(workspace, alias); err != nil {
		t.Fatal(err)
	}
	c, err := OpenCatalogue(root, workspace)
	if err != nil {
		t.Fatal(err)
	}
	other, err := OpenCatalogue(root, alias)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Register(catalogueRecord("one"), true); err != nil {
		t.Fatal(err)
	}
	s, err := other.Snapshot()
	if err != nil || len(s.Sessions) != 1 || c.dir != other.dir {
		t.Fatal("alias got separate catalogue", err)
	}
}

func TestCatalogueRejectsInvalidMutationsWithoutChangingDisk(t *testing.T) {
	c := catalogueFixture(t)
	if err := c.Register(catalogueRecord("one"), true); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(c.dir, "catalogue.json"))
	invalid := catalogueRecord("../outside")
	rebound := catalogueRecord("one")
	rebound.StoreKey = "different"
	duplicateKey := catalogueRecord("two")
	duplicateKey.StoreKey = "key_one"
	for _, err := range []error{c.Register(invalid, true), c.Register(rebound, false), c.Register(duplicateKey, false), c.Select("missing"), c.Rename("one", "\x1b[31m"), c.Rename("missing", "name")} {
		if err == nil {
			t.Fatal("invalid mutation accepted")
		}
	}
	after, _ := os.ReadFile(filepath.Join(c.dir, "catalogue.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("rejected mutation changed file")
	}
}

func TestCatalogueLockPreventsLostUpdate(t *testing.T) {
	c := catalogueFixture(t)
	if err := c.Register(catalogueRecord("one"), true); err != nil {
		t.Fatal(err)
	}
	lock, err := lockCatalogue(filepath.Join(c.dir, "catalogue.lock"))
	if err != nil {
		t.Fatal(err)
	}
	other := &Catalogue{dir: c.dir, workspace: c.workspace}
	if err := other.Register(catalogueRecord("two"), true); !errors.Is(err, ErrCatalogueBusy) {
		t.Fatal("competing writer acquired lock", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err := other.Register(catalogueRecord("two"), true); err != nil {
		t.Fatal(err)
	}
	s, err := c.Snapshot()
	if err != nil || len(s.Sessions) != 2 {
		t.Fatal("second writer lost first record", err)
	}
}

func TestCatalogueRejectsFutureCorruptAndWrongWorkspace(t *testing.T) {
	c := catalogueFixture(t)
	if err := os.MkdirAll(c.dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"version":2}`, `{"version":1,"workspace":"wrong","sessions":[]}`, `{"version":1`, "null", `{} {}`} {
		path := filepath.Join(c.dir, "catalogue.json")
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Snapshot(); err == nil {
			t.Fatal("invalid catalogue loaded")
		}
		if err := c.Register(catalogueRecord("new"), true); err == nil {
			t.Fatal("invalid catalogue overwritten")
		}
		after, _ := os.ReadFile(path)
		if string(after) != raw {
			t.Fatal("corrupt catalogue modified")
		}
	}
}

func TestCatalogueRejectsSymlinkMetadata(t *testing.T) {
	c := catalogueFixture(t)
	if err := os.MkdirAll(c.dir, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(c.dir, "catalogue.json")); err != nil {
		t.Fatal(err)
	}
	if err := c.Register(catalogueRecord("one"), true); err == nil {
		t.Fatal("symlink catalogue accepted")
	}
	raw, _ := os.ReadFile(target)
	if string(raw) != "untouched" {
		t.Fatal("symlink target modified")
	}
}

func TestCatalogueAnchorsRelativeStoreRoot(t *testing.T) {
	root := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, root)
	if err != nil {
		t.Fatal(err)
	}
	c, err := OpenCatalogue(relative, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(c.dir) {
		t.Fatal("relative catalogue changes with process working directory")
	}
}
