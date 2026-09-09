package app

import (
	"context"
	"errors"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tool/skills"
	"strings"
	"testing"
)

func TestReloadSkillsOwnedAndCancelled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	provider, store, err := agentio.BuildSkillProvider(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rt, err := runtime.BuildRuntime(runtime.RuntimeDeps{Skills: provider}, runtime.RuntimeInputs{Tools: tool.NewRegistry()}, runtime.AgentSpec{SystemPrompt: "identity"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	c := &Controller{Rt: rt}
	if _, err = store.Create(ctx, skills.Skill{Name: "created", Body: "new procedure"}); err != nil {
		t.Fatal(err)
	}
	_, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		t.Fatal(err)
	}
	err = c.ReloadSkills(ctx)
	release()
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("busy reload: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err = c.ReloadSkills(cancelled); err == nil {
		t.Fatal("cancelled reload accepted")
	}
	if strings.Contains(rt.StaticSystemPrompt, "created") {
		t.Fatal("rejected reload changed prompt")
	}
	if err = c.ReloadSkills(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rt.StaticSystemPrompt, "created") {
		t.Fatal("reload missing authored skill")
	}
}
