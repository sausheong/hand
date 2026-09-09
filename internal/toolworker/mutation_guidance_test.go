package toolworker

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkerMutationRequiresCurrentNestedGuidance(t *testing.T) {
	work := t.TempDir()
	os.Mkdir(filepath.Join(work, "sub"), 0700)
	os.WriteFile(filepath.Join(work, "sub", "HAND.md"), []byte("Do not change public names"), 0600)
	input := map[string]any{"path": "sub/file", "content": "bounded change"}
	call := func() Response {
		t.Helper()
		raw, _ := json.Marshal(input)
		request, _ := json.Marshal(Request{Version: 1, Tool: "write_file", Input: raw})
		var out bytes.Buffer
		if err := Serve(context.Background(), work, bytes.NewReader(request), &out); err != nil {
			t.Fatal(err)
		}
		var response Response
		if err := json.Unmarshal(out.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	first := call()
	if first.Result.Error == "" {
		t.Fatal("worker wrote before guidance")
	}
	if _, err := os.Stat(filepath.Join(work, "sub/file")); !os.IsNotExist(err) {
		t.Fatal("first call mutated")
	}
	input["instruction_digest"] = first.Result.Metadata["instruction_digest"]
	second := call()
	if second.Result.Error != "" {
		t.Fatal(second.Result.Error)
	}
	body, err := os.ReadFile(filepath.Join(work, "sub/file"))
	if err != nil || string(body) != "bounded change" {
		t.Fatalf("worker write %q %v", body, err)
	}
}
