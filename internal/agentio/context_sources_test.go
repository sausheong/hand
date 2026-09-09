package agentio_test

import (
	"context"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tool/skills"
	"os"
	"path/filepath"
	"testing"
)

func TestContextSourcesMatchInstalledPrompt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	work := t.TempDir()
	path := filepath.Join(work, "HAND.md")
	if err := os.WriteFile(path, []byte("12345678"), 0600); err != nil {
		t.Fatal(err)
	}
	p, store, err := agentio.BuildSkillProvider(work)
	if err != nil {
		t.Fatal(err)
	}
	spec := agentio.BuildAgentSpec("local/test", work, 2, "", nil, runtime.LifecycleHooks{})
	rt, err := runtime.BuildRuntime(runtime.RuntimeDeps{Skills: p}, runtime.RuntimeInputs{Tools: tool.NewRegistry()}, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	spec.SystemPromptSources[0].Path = "caller mutation"
	if err := os.WriteFile(path, []byte("changed body after startup"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(context.Background(), skills.Skill{Name: "source-skill", Body: "---\ndescription: a workflow\n---\nbody"}); err != nil {
		t.Fatal(err)
	}
	before, err := rt.InspectContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Sources) != 1 || before.Sources[0].Path != path || before.Sources[0].EstimatedTokens != 2 {
		t.Fatalf("snapshot: %+v", before.Sources)
	}
	before.Sources[0].Path = "returned mutation"
	if err := rt.RefreshSkills(); err != nil {
		t.Fatal(err)
	}
	after, err := rt.InspectContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Sources) != 2 || after.Sources[0].Path != path || after.Sources[1].Kind != "skill_index" || after.Sources[1].Path != filepath.Join(work, ".hand", "skills", "source-skill", "SKILL.md") {
		t.Fatalf("refreshed sources: %+v", after.Sources)
	}
}
