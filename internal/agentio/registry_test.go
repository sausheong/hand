package agentio_test

import (
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/tool/skills/disk"
)

func TestBuildRegistry_RegistersExpectedTools(t *testing.T) {
	reg := agentio.BuildRegistry(t.TempDir(), disk.NewStore(t.TempDir()))

	got := map[string]bool{}
	for _, def := range reg.ToolDefs() {
		got[def.Name] = true
	}

	want := []string{
		"read_file", "write_file", "edit_file",
		"bash", "web_fetch", "web_search", "todo_write", "search", "skill_manage",
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("registry missing tool %q", name)
		}
	}
	if len(got) != len(want) {
		t.Errorf("registry has %d tools, want exactly %d (%v)", len(got), len(want), want)
	}
}
