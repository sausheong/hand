package agentio_test

import (
	"context"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/tool/skills"
	"github.com/sausheong/harness/tool/skills/disk"
)

func TestBuildSkillProvider_EmptyBothYieldsEmptyIndex(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace := t.TempDir()

	provider, _, err := agentio.BuildSkillProvider(workspace)
	if err != nil {
		t.Fatalf("BuildSkillProvider returned error: %v", err)
	}
	if got := provider.FormatIndex(); got != "" {
		t.Fatalf("FormatIndex() = %q, want empty when neither store has skills", got)
	}
	if _, ok := provider.Get("anything"); ok {
		t.Fatal("Get should report not-found when neither store has skills")
	}
}

func TestBuildSkillProvider_ProjectOverridesGlobalByName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspace := t.TempDir()

	globalDir, err := agentio.GlobalSkillsDir()
	if err != nil {
		t.Fatalf("GlobalSkillsDir returned error: %v", err)
	}
	globalStore := disk.NewStore(globalDir)
	if _, err := globalStore.Create(context.Background(), skills.Skill{
		Name: "run-tests",
		Body: "---\ndescription: global version\n---\n\nglobal body",
	}); err != nil {
		t.Fatalf("Create (global) returned error: %v", err)
	}

	projectStore := disk.NewStore(agentio.ProjectSkillsDir(workspace))
	if _, err := projectStore.Create(context.Background(), skills.Skill{
		Name: "run-tests",
		Body: "---\ndescription: project version\n---\n\nproject body",
	}); err != nil {
		t.Fatalf("Create (project) returned error: %v", err)
	}

	provider, projectFromBuild, err := agentio.BuildSkillProvider(workspace)
	if err != nil {
		t.Fatalf("BuildSkillProvider returned error: %v", err)
	}
	if projectFromBuild == nil {
		t.Fatal("BuildSkillProvider returned a nil project store")
	}

	index := provider.FormatIndex()
	if !strings.Contains(index, "run-tests") || !strings.Contains(index, "project version") {
		t.Fatalf("FormatIndex() = %q, want the project description to win", index)
	}
	if strings.Contains(index, "global version") {
		t.Fatalf("FormatIndex() = %q, want the global description shadowed", index)
	}

	body, ok := provider.Get("run-tests")
	if !ok {
		t.Fatal("Get(\"run-tests\") not found, want the project version")
	}
	if !strings.Contains(body, "project body") {
		t.Fatalf("Get(\"run-tests\") = %q, want the project body", body)
	}
}

func TestBuildSkillProvider_FallsBackToGlobalWhenNotInProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspace := t.TempDir()

	globalDir, err := agentio.GlobalSkillsDir()
	if err != nil {
		t.Fatalf("GlobalSkillsDir returned error: %v", err)
	}
	globalStore := disk.NewStore(globalDir)
	if _, err := globalStore.Create(context.Background(), skills.Skill{
		Name: "personal-only",
		Body: "---\ndescription: personal\n---\n\nbody",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	provider, _, err := agentio.BuildSkillProvider(workspace)
	if err != nil {
		t.Fatalf("BuildSkillProvider returned error: %v", err)
	}
	if _, ok := provider.Get("personal-only"); !ok {
		t.Fatal("Get(\"personal-only\") not found, want fallback to the global store")
	}
}
