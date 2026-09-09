package agentio

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMutationGuidanceBeforeWriteAndStaleDigest(t *testing.T) {
	for _, exact := range []bool{false, true} {
		work := t.TempDir()
		if err := os.MkdirAll(filepath.Join(work, "sub"), 0700); err != nil {
			t.Fatal(err)
		}
		guidance := filepath.Join(work, "sub", "AGENTS.md")
		if err := os.WriteFile(guidance, []byte("Keep generated files deterministic"), 0600); err != nil {
			t.Fatal(err)
		}
		writer := WriteFileWithInstructions(work, exact)
		input := map[string]any{"path": "sub/new/deep/file", "content": "first"}
		call := func() (string, string) {
			t.Helper()
			raw, _ := json.Marshal(input)
			result, err := writer.Execute(context.Background(), raw)
			if err != nil {
				t.Fatal(err)
			}
			digest, _ := result.Metadata["instruction_digest"].(string)
			return result.Error, digest
		}
		reason, digest := call()
		if reason == "" || len(digest) != 64 {
			t.Fatal("mutation did not return guidance")
		}
		target := filepath.Join(work, "sub/new/deep/file")
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatal("file created before guidance")
		}
		if err := os.WriteFile(guidance, []byte("Updated constraint"), 0600); err != nil {
			t.Fatal(err)
		}
		input["instruction_digest"] = digest
		reason, newDigest := call()
		if reason == "" || newDigest == digest {
			t.Fatal("stale guidance accepted")
		}
		input["instruction_digest"] = newDigest
		if reason, _ = call(); reason != "" {
			t.Fatal(reason)
		}
		body, err := os.ReadFile(target)
		if err != nil || string(body) != "first" {
			t.Fatalf("write %q %v", body, err)
		}
		editor := EditFileWithInstructions(work, exact)
		raw, _ := json.Marshal(map[string]any{"path": "sub/new/deep/file", "old_string": "first", "new_string": "second"})
		result, err := editor.Execute(context.Background(), raw)
		if err != nil || result.Error == "" || !strings.Contains(result.Output, "Updated constraint") {
			t.Fatal("edit missed guidance", result, err)
		}
		raw, _ = json.Marshal(map[string]any{"path": "sub/new/deep/file", "old_string": "first", "new_string": "second", "instruction_digest": result.Metadata["instruction_digest"]})
		result, err = editor.Execute(context.Background(), raw)
		if err != nil || result.Error != "" {
			t.Fatal(result, err)
		}
		body, err = os.ReadFile(target)
		if err != nil || string(body) != "second" {
			t.Fatalf("edit %q %v", body, err)
		}
	}
}
func TestMutationGuidanceDoesNotExpandWorkspaceAuthority(t *testing.T) {
	work, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "HAND.md"), []byte("outside secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(work, "escape")); err != nil {
		t.Fatal(err)
	}
	result, err := WriteFileWithInstructions(work, true).Execute(context.Background(), json.RawMessage(`{"path":"escape/file","content":"x"}`))
	if err == nil && result.Error == "" {
		t.Fatal("outside write allowed")
	}
	if strings.Contains(result.Output, "outside secret") {
		t.Fatal("outside guidance leaked")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = WriteFileWithInstructions(work, true).Execute(ctx, json.RawMessage(`{"path":"file","content":"x"}`))
	if err == nil {
		t.Fatal("cancelled mutation accepted")
	}
	if _, err = os.Stat(filepath.Join(work, "file")); !os.IsNotExist(err) {
		t.Fatal("cancelled write happened")
	}
}
