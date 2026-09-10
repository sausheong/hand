package agentio

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNestedReadGuidanceScopeAndPrecedence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	work := t.TempDir()
	files := map[string]string{"HAND.md": "root not repeated", "sub/AGENTS.md": "broad nested guidance", "sub/deep/HAND.md": "nearest guidance", "sub/deep/AGENTS.md": "shadow body", "sibling/HAND.md": "unrelated guidance", "sub/deep/code.txt": "source contents"}
	for rel, body := range files {
		path := filepath.Join(work, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, exact := range []bool{false, true} {
		reader := ReadFileWithInstructions(work, exact)
		result, err := reader.Execute(context.Background(), json.RawMessage(`{"path":"sub/deep/code.txt"}`))
		if err != nil || result.Error != "" {
			t.Fatalf("read: %+v %v", result, err)
		}
		for _, want := range []string{"broad nested guidance", "nearest guidance", "source contents", "shadowed by"} {
			if !strings.Contains(result.Output, want) {
				t.Fatalf("missing %s: %s", want, result.Output)
			}
		}
		for _, unwanted := range []string{"root not repeated", "unrelated guidance", "shadow body"} {
			if strings.Contains(result.Output, unwanted) {
				t.Fatal("inapplicable guidance loaded", unwanted)
			}
		}
		if strings.Index(result.Output, "broad nested guidance") > strings.Index(result.Output, "nearest guidance") {
			t.Fatal("precedence reversed")
		}
		sources, ok := result.Metadata["instruction_sources"].([]InstructionSource)
		if !ok || len(sources) != 2 {
			t.Fatal("provenance missing")
		}
	}
}

func TestNestedReadRejectsOutsideAndRefreshes(t *testing.T) {
	work := t.TempDir()
	outside := t.TempDir()
	for _, dir := range []string{work, outside} {
		if err := os.WriteFile(filepath.Join(dir, "file"), []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "HAND.md"), []byte("outside secret"), 0600); err != nil {
		t.Fatal(err)
	}
	r := DiscoverNestedInstructions(work, filepath.Join(outside, "file"))
	if len(r.Sources) != 0 {
		t.Fatal("outside guidance read")
	}
	if err := os.Mkdir(filepath.Join(work, "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(work, "sub", "file")
	os.WriteFile(path, []byte("data"), 0600)
	if len(DiscoverNestedInstructions(work, path).Sources) != 0 {
		t.Fatal("premature guidance")
	}
	os.WriteFile(filepath.Join(work, "sub", "HAND.md"), []byte("new guidance"), 0600)
	if got := DiscoverNestedInstructions(work, path); len(got.Sources) != 1 || got.Sources[0].Body != "new guidance" {
		t.Fatalf("refresh: %+v", got)
	}
	result, err := ReadFileWithInstructions(work, true).Execute(context.Background(), json.RawMessage(`{"path":"missing"}`))
	if err == nil && result.Error == "" {
		t.Fatal("missing read accepted")
	}
	if result.Metadata["instruction_sources"] != nil {
		t.Fatal("failed read loaded guidance")
	}
}
