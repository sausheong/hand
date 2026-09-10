package verification

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordSaveLoadIntegrity(t *testing.T) {
	o := options(t, "printf verified")
	r, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	id, err := r.Save(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := r.Save(context.Background(), dir); err != nil || again != id {
		t.Fatal(again, err)
	}
	loaded, err := Load(context.Background(), dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.StatusFor(current(t, o), o.ProfileDigest) != "passed" || loaded.View().Stdout != "verified" {
		t.Fatal(loaded.View())
	}
	file := filepath.Join(dir, id+".json")
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	b[len(b)/2] ^= 1
	if err = os.WriteFile(file, b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(context.Background(), dir, id); err == nil {
		t.Fatal("tampered record accepted")
	}
	if _, err = r.Save(context.Background(), dir); err == nil {
		t.Fatal("idempotent save masked corruption")
	}
}
func TestRecordSaveScopeCancellationAndSymlink(t *testing.T) {
	o := options(t, "exit 7")
	r, _ := Run(context.Background(), o)
	if r == nil {
		t.Fatal("missing failure record")
	}
	if _, err := r.Save(context.Background(), o.Workspace); err == nil {
		t.Fatal("record saved inside workspace")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Save(ctx, dir); err == nil {
		t.Fatal("cancel ignored")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("cancelled save created state")
	}
	id, err := r.Save(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(context.Background(), dir, id)
	if err != nil || loaded.Status(nil) != "failed" {
		t.Fatal(loaded, err)
	}
	file := filepath.Join(dir, id+".json")
	if err = os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(filepath.Join(o.Workspace, "input"), file); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(context.Background(), dir, id); err == nil {
		t.Fatal("symlink followed")
	}
}

func TestRecordBinaryOutputRoundtrip(t *testing.T) {
	o := options(t, "printf '\\377'")
	r, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	id, err := r.Save(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(context.Background(), dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.View().Stdout != string([]byte{255}) {
		t.Fatal("binary output changed")
	}
}
