package verification

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEvidenceCatalogueAndSelectedDeletion(t *testing.T) {
	ctx := context.Background()
	o := options(t, "printf evidence")
	r, err := Run(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	first, err := r.Save(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	r.view.Finished = r.view.Finished.Add(time.Second)
	second, err := r.Save(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	page, err := List(ctx, dir, 0)
	if err != nil || page.Total != 2 || page.Next != 2 || len(page.IDs) != 2 || page.IDs[0] >= page.IDs[1] {
		t.Fatal(page, err)
	}
	if _, err := List(ctx, dir, 3); err == nil {
		t.Fatal("invalid offset accepted")
	}
	if err := Delete(ctx, dir, first, t.TempDir()); err == nil {
		t.Fatal("wrong workspace deletion accepted")
	}
	if err := Delete(ctx, dir, first, o.Workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(ctx, dir, first); !os.IsNotExist(err) {
		t.Fatal("selected record remains", err)
	}
	if _, err := Load(ctx, dir, second); err != nil {
		t.Fatal("unselected record lost", err)
	}
	if _, err := o.Checkpoints.Load(ctx, r.view.Before); err != nil {
		t.Fatal("snapshot deleted", err)
	}
	if err := os.WriteFile(filepath.Join(dir, second+".json"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Delete(ctx, dir, second, o.Workspace); err == nil {
		t.Fatal("corrupt evidence silently deleted")
	}
	if _, err := os.Stat(filepath.Join(dir, second+".json")); err != nil {
		t.Fatal("corrupt evidence lost", err)
	}
}
