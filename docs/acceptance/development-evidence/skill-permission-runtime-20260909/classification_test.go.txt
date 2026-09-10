package agentio

import (
	"github.com/sausheong/harness/tool/skills/disk"
	"testing"
)

func TestEveryRegisteredToolHasExplicitEffectClassification(t *testing.T) {
	reg := BuildRegistry(t.TempDir(), disk.NewStore(t.TempDir()))
	for _, def := range reg.ToolDefs() {
		_, gated := gatedTools[def.Name]
		_, read := readOnlyTools[def.Name]
		if gated == read {
			t.Errorf("tool %s must have exactly one classification", def.Name)
		}
	}
	if !readOnlyTools["load_skill"] {
		t.Fatal("Harness load_skill must be classified")
	}
}
