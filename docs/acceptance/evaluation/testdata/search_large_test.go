package agentio_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
)

func TestAcceptanceLargeTextSearch(t *testing.T) {
	dir := t.TempDir()
	for _, lines := range []int{12000, 30000} {
		name := fmt.Sprintf("source-%d.txt", lines)
		payload := strings.Repeat("ordinary source line\n", lines) + "acceptance_needlex\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(payload), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// The filename filter still applies; larger files must not bypass it.
	if err := os.WriteFile(filepath.Join(dir, "excluded.log"), []byte("acceptance_needlex\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := (&agentio.SearchTool{WorkDir: dir}).Execute(context.Background(), json.RawMessage(`{"content":"acceptance_needlex","name_glob":"*.txt","max_results":10}`))
	if err != nil || result.Error != "" {
		t.Fatalf("search failed: %+v %v", result, err)
	}
	for _, lines := range []int{12000, 30000} {
		want := fmt.Sprintf("source-%d.txt:%d:acceptance_needlex", lines, lines+1)
		if !strings.Contains(result.Output, want) {
			t.Errorf("missing actual match %q in %q", want, result.Output)
		}
	}
	if strings.Contains(result.Output, "excluded.log") {
		t.Fatal("filename filter ignored")
	}
	capped, err := (&agentio.SearchTool{WorkDir: dir}).Execute(context.Background(), json.RawMessage(`{"content":"acceptance_needlex","name_glob":"*.txt","max_results":1}`))
	if err != nil || capped.Error != "" || strings.Count(capped.Output, ":acceptance_needlex") != 1 {
		t.Fatalf("result cap lost: %+v %v", capped, err)
	}
}
