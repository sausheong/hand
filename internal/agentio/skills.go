package agentio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool/skills"
	"github.com/sausheong/harness/tool/skills/disk"
)

// GlobalSkillsDir returns ~/.hand/skills for the current user — personal
// skills the user maintains by hand, shared across every project.
func GlobalSkillsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".hand", "skills"), nil
}

// ProjectSkillsDir returns workspace/.hand/skills — project-specific
// skills that can be committed to the repo alongside HAND.md/AGENTS.md,
// and the store the agent's own skill_manage calls write to (see
// BuildSkillProvider).
func ProjectSkillsDir(workspace string) string {
	return filepath.Join(workspace, ".hand", "skills")
}

// mergedSkillProvider satisfies runtime.SkillProvider by combining a
// personal (global) and a project-local skill store. A project skill
// shadows a global skill of the same name — the workspace's own
// procedural knowledge is assumed more relevant than whatever a
// same-named personal skill says.
type mergedSkillProvider struct {
	global  *disk.Store
	project *disk.Store
}

// FormatIndex implements runtime.SkillProvider. Returns a single
// "## Skills" markdown block listing every skill from both stores, one
// bullet per name (project entries winning any name collision), sorted.
// Empty when neither store has any skills.
func (m *mergedSkillProvider) FormatIndex() string {
	byName := make(map[string]skills.Skill)

	if globalSkills, err := m.global.List(context.Background()); err == nil {
		for _, sk := range globalSkills {
			byName[sk.Name] = sk
		}
	}
	if projectSkills, err := m.project.List(context.Background()); err == nil {
		for _, sk := range projectSkills {
			byName[sk.Name] = sk
		}
	}
	if len(byName) == 0 {
		return ""
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("## Skills\n\n")
	for _, name := range names {
		sk := byName[name]
		b.WriteString("- ")
		b.WriteString(sk.Name)
		if sk.Description != "" {
			b.WriteString(": ")
			b.WriteString(sk.Description)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// Get implements runtime.SkillProvider. Checks the project store first
// so a project skill's body is what load_skill returns even when a
// same-named global skill also exists.
func (m *mergedSkillProvider) Get(name string) (string, bool) {
	if sk, ok, err := m.project.Get(context.Background(), name); err == nil && ok {
		return sk.Body, true
	}
	if sk, ok, err := m.global.Get(context.Background(), name); err == nil && ok {
		return sk.Body, true
	}
	return "", false
}

// BuildSkillProvider constructs hand's skill stores for workspace and
// returns the merged runtime.SkillProvider (for RuntimeDeps.Skills) plus
// the project-local store (for registering skill_manage — see
// registry.go). The agent's self-authoring writes are scoped to the
// project store only: an agent-authored skill stays with the repo it
// was learned in rather than silently spreading into the user's
// personal ~/.hand/skills/.
func BuildSkillProvider(workspace string) (runtime.SkillProvider, *disk.Store, error) {
	globalDir, err := GlobalSkillsDir()
	if err != nil {
		return nil, nil, err
	}
	global := disk.NewStore(globalDir)
	project := disk.NewStore(ProjectSkillsDir(workspace))
	return &mergedSkillProvider{global: global, project: project}, project, nil
}
