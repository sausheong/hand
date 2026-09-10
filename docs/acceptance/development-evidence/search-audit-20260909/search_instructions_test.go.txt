package agentio

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchNestedGuidanceScopeAndBounds(t *testing.T) {
	root := t.TempDir()
	put := func(name, body string) {
		t.Helper()
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	put("nested/HAND.md", "Use the nested API.")
	put("nested/AGENTS.md", "SHADOWED RULE")
	put("sibling/HAND.md", "UNRELATED RULE")
	put("nested/a.go", "needle\nneedle\n")
	tool := &SearchTool{WorkDir: root}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"content":"needle","name_glob":"*.go"}`))
	if err != nil || result.Error != "" {
		t.Fatalf("%+v %v", result, err)
	}
	if strings.Count(result.Output, "Use the nested API.") != 1 || strings.Contains(result.Output, "SHADOWED RULE") || strings.Contains(result.Output, "UNRELATED RULE") {
		t.Fatal(result.Output)
	}
	// Collision diagnostics are delivered with the guidance, not omitted.
	if result.Metadata["instruction_guidance_complete"] != true {
		t.Fatal(result.Metadata)
	}
	put("nested/HAND.md", strings.Repeat("Long guidance. ", 300))
	result, err = tool.Execute(context.Background(), json.RawMessage(`{"content":"needle","name_glob":"*.go","max_bytes":2048}`))
	if err != nil || len(result.Output) > 2048 || result.Metadata["instruction_guidance_complete"] != false || !strings.Contains(result.Output, "guidance was omitted") || strings.Contains(result.Output, "Long guidance.") {
		t.Fatalf("%+v %v", result, err)
	}
	if result.Metadata["complete"] != true {
		t.Fatal("guidance limit must not claim matches were omitted")
	}
}
