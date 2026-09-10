package toolproxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspacePathMappingPreservesContentAndRejectsEscape(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err = os.Symlink(outside, filepath.Join(workspace, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"nested/file", filepath.Join(workspace, "nested/file")} {
		raw, _ := json.Marshal(map[string]string{"path": path, "content": outside})
		mapped, err := mapInput(workspace, "write_file", raw)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]string
		json.Unmarshal(mapped, &fields)
		if fields["path"] != "/workspace/nested/file" || fields["content"] != outside {
			t.Fatal(string(mapped))
		}
	}
	for _, path := range []string{outside, "../outside", "escape/file", workspace + "-other/file"} {
		raw, _ := json.Marshal(map[string]string{"path": path})
		if _, err = mapInput(workspace, "read_file", raw); err == nil {
			t.Fatalf("escape accepted: %s", path)
		}
	}
	raw, err := mapInput(workspace, "search", json.RawMessage(`{"pattern":"needle"}`))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]string
	json.Unmarshal(raw, &fields)
	if fields["path"] != "/workspace/." {
		t.Fatal(string(raw))
	}
}
