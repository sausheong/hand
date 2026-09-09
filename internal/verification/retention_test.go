package verification

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVerificationRetentionPreservesAcceptedRecords(t *testing.T) {
	o := options(t, "printf evidence")
	r, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	limits := RetentionLimits{MaxRecords: 1, MaxBytes: 1 << 20}
	id, err := r.SaveWithLimits(context.Background(), dir, limits)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := r.SaveWithLimits(context.Background(), dir, limits); err != nil || again != id {
		t.Fatal("idempotent save rejected at capacity", again, err)
	}
	changed := &Record{view: r.View()}
	changed.view.Finished = changed.view.Finished.Add(time.Second)
	if _, err := changed.SaveWithLimits(context.Background(), dir, limits); err == nil {
		t.Fatal("record capacity ignored")
	}
	if _, err := Load(context.Background(), dir, id); err != nil {
		t.Fatal("accepted evidence lost", err)
	}
	if _, err := changed.SaveWithLimits(context.Background(), dir, RetentionLimits{MaxRecords: 2, MaxBytes: 1}); err == nil {
		t.Fatal("byte capacity ignored")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	lock, err := lockEvidence(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := changed.SaveWithLimits(context.Background(), dir, RetentionLimits{MaxRecords: 2, MaxBytes: 1 << 20}); err == nil {
		t.Fatal("concurrent writer admitted")
	}
	lock.Close()
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
}
