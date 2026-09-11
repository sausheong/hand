package agentio_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/tool"
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

func TestSkillConventionalPathsProvenanceAndRefresh(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspace := t.TempDir()
	provider, writer, err := agentio.BuildSkillProvider(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if provider.FormatIndex() != "" {
		t.Fatal("unexpected initial skills")
	}
	for _, root := range []string{filepath.Join(home, ".agents", "skills"), filepath.Join(workspace, ".agents", "skills")} {
		dir := filepath.Join(root, "conventional")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\ndescription: conventional guide\n---\nRead references/guide.md"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	index := provider.FormatIndex()
	if !strings.Contains(index, "shadowed by") || !strings.Contains(index, "conventional guide") {
		t.Fatal(index)
	}
	body, ok := provider.Get("conventional")
	if !ok || !strings.Contains(body, "Resource base directory: "+filepath.Join(workspace, ".agents", "skills", "conventional")) {
		t.Fatal(body)
	}
	if _, err := writer.Create(context.Background(), skills.Skill{Name: "conventional", Body: "authored hand body"}); err != nil {
		t.Fatal(err)
	}
	body, ok = provider.Get("conventional")
	if !ok || !strings.Contains(body, "authored hand body") || strings.Contains(body, "Read references") {
		t.Fatal(body)
	}
	if _, err := writer.Create(context.Background(), skills.Skill{Name: "new-skill", Body: "new body"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(provider.FormatIndex(), "new-skill") {
		t.Fatal("authored skill missing from refreshed index")
	}
}

func TestSkillDiscoveryBoundsVisible(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace := t.TempDir()
	dir := filepath.Join(workspace, ".hand", "skills", "huge")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(strings.Repeat("x", (1<<20)+1)), 0600); err != nil {
		t.Fatal(err)
	}
	p, _, err := agentio.BuildSkillProvider(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.FormatIndex(), "exceeds 1 MiB") {
		t.Fatal(p.FormatIndex())
	}
	if _, ok := p.Get("huge"); ok {
		t.Fatal("oversized skill loaded")
	}
	if _, ok := p.Get("../huge"); ok {
		t.Fatal("invalid name loaded")
	}
}

func TestLargeSkillIndexedAndLoadedCompletely(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace := t.TempDir()
	dir := filepath.Join(workspace, ".agents", "skills", "large")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "SKILL.md")
	body := "---\nname: large\ndescription: Large skill fixture\n---\n" + strings.Repeat("full instructions\n", 6000) + "FINAL INSTRUCTION\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	p, _, err := agentio.BuildSkillProvider(workspace)
	if err != nil {
		t.Fatal(err)
	}
	index := p.FormatIndex()
	if !strings.Contains(index, "Large skill fixture") || strings.Contains(index, "FINAL INSTRUCTION") || strings.Contains(index, "exceeds") {
		t.Fatal(index)
	}
	loader := &tool.LoadSkillTool{Lookup: p.Get}
	result, err := loader.Execute(context.Background(), []byte(`{"name":"large"}`))
	if err != nil || !strings.HasSuffix(result.Output, body) {
		t.Fatal("skill instructions were lost", err)
	}
	// Loading must re-read the selected file rather than return stale discovery data.
	body += "UPDATED\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, ok := p.Get("large")
	if !ok || !strings.HasSuffix(loaded, body) {
		t.Fatal("stale body")
	}
}
