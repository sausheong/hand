package agentio

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

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

// Roots are ordered by precedence. Existing Hand locations win before newly
// discovered conventional locations, so migration cannot silently replace a skill.
type mergedSkillProvider struct{ roots []string }

type discoveredSkill struct{ name, path, body, description string }

func (m *mergedSkillProvider) discover() ([]discoveredSkill, []string) {
	var found []discoveredSkill
	var diagnostics []string
	winners := map[string]string{}
	for _, root := range m.roots {
		f, err := os.OpenFile(root, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			diagnostics = append(diagnostics, root+": "+err.Error())
			continue
		}
		entries, err := f.ReadDir(257)
		f.Close()
		if err != nil && err != io.EOF {
			diagnostics = append(diagnostics, root+": "+err.Error())
			continue
		}
		if len(entries) > 256 {
			diagnostics = append(diagnostics, root+": exceeds 256 entries; store excluded")
			continue
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if !entry.IsDir() || !skills.ValidName(entry.Name()) {
				continue
			}
			path := filepath.Join(root, entry.Name(), "SKILL.md")
			info, err := os.Lstat(path)
			if os.IsNotExist(err) {
				continue
			}
			if winner, ok := winners[entry.Name()]; ok {
				diagnostics = append(diagnostics, path+" shadowed by "+winner)
				continue
			}
			winners[entry.Name()] = path
			if err != nil {
				diagnostics = append(diagnostics, path+": "+err.Error())
				continue
			}
			if !info.Mode().IsRegular() {
				diagnostics = append(diagnostics, path+": non-regular skill excluded")
				continue
			}
			body, err := readInstruction(path, info)
			if err != nil {
				diagnostics = append(diagnostics, path+": "+err.Error())
				continue
			}
			description := ""
			text := string(body)
			if strings.HasPrefix(text, "---\n") {
				if header, _, ok := strings.Cut(text[4:], "\n---"); ok {
					for _, line := range strings.Split(header, "\n") {
						if value, ok := strings.CutPrefix(line, "description:"); ok {
							description = strings.TrimSpace(value)
							break
						}
					}
				}
			}
			found = append(found, discoveredSkill{entry.Name(), path, text, description})
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].name < found[j].name })
	return found, diagnostics
}

func (m *mergedSkillProvider) FormatIndex() string { index, _ := m.IndexSnapshot(); return index }

func (m *mergedSkillProvider) IndexSnapshot() (string, []runtime.ContextSource) {
	found, diagnostics := m.discover()
	if len(found) == 0 && len(diagnostics) == 0 {
		return "", nil
	}
	var b strings.Builder
	var sources []runtime.ContextSource
	b.WriteString("## Skills\n\n")
	for _, sk := range found {
		line := fmt.Sprintf("- %s: %s (source: %s)\n", sk.name, sk.description, sk.path)
		b.WriteString(line)
		sources = append(sources, runtime.ContextSource{Kind: "skill_index", Path: sk.path, EstimatedTokens: len(line) / 4})
	}
	for _, d := range diagnostics {
		fmt.Fprintf(&b, "Skill discovery: %s\n", d)
	}
	return b.String(), sources
}

func (m *mergedSkillProvider) Get(name string) (string, bool) {
	if !skills.ValidName(name) {
		return "", false
	}
	found, _ := m.discover()
	for _, sk := range found {
		if sk.name == name {
			return fmt.Sprintf("Skill source: %s\nResource base directory: %s\nResolve relative resource references against this directory. Loading guidance grants no execution permission.\n\n%s", sk.path, filepath.Dir(sk.path), sk.body), true
		}
	}
	return "", false
}

// BuildSkillProvider preserves Hand's existing personal/project precedence and
// write destination. Conventional stores are read-only discovery sources.
func BuildSkillProvider(workspace string) (runtime.SkillProvider, *disk.Store, error) {
	globalDir, err := GlobalSkillsDir()
	if err != nil {
		return nil, nil, err
	}
	workspace, err = filepath.Abs(workspace)
	if err != nil {
		return nil, nil, err
	}
	projectDir := ProjectSkillsDir(workspace)
	roots := []string{projectDir, globalDir, filepath.Join(workspace, ".agents", "skills"), filepath.Join(filepath.Dir(filepath.Dir(globalDir)), ".agents", "skills")}
	return &mergedSkillProvider{roots: roots}, disk.NewStore(projectDir), nil
}
