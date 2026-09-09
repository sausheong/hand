package file

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAcceptanceWriteMode(t *testing.T) {
	for _, mode := range []os.FileMode{0600, 0640, 0751} {
		dir := t.TempDir()
		path := filepath.Join(dir, "script.sh")
		if err := os.WriteFile(path, []byte("original"), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		input, _ := json.Marshal(map[string]string{"path": path, "content": "replacement"})
		result, err := (&WriteFileTool{WorkDir: dir}).Execute(context.Background(), input)
		if err != nil || result.Error != "" {
			t.Fatalf("write failed: %v %s", err, result.Error)
		}
		content, err := os.ReadFile(path)
		if err != nil || string(content) != "replacement" {
			t.Fatalf("replacement absent: %q %v", content, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != mode {
			t.Errorf("overwrite changed mode %04o to %04o", mode, info.Mode().Perm())
		}
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")
	input, _ := json.Marshal(map[string]string{"path": path, "content": "new"})
	result, err := (&WriteFileTool{WorkDir: dir}).Execute(context.Background(), input)
	if err != nil || result.Error != "" {
		t.Fatalf("new file failed: %v %s", err, result.Error)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("new file permissions %04o", info.Mode().Perm())
	}
}
