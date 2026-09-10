package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sausheong/hand/internal/packages"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tool/skills"
)

type PackageSkillSelection struct {
	Package string `json:"package"`
	Digest  string `json:"digest"`
	Path    string `json:"path"`
	Name    string `json:"name"`
}
type packageSkillEntry struct {
	name     string
	resource packages.TextResource
}
type packageSkills struct {
	base    runtime.SkillProvider
	entries []packageSkillEntry
}

func packageSkillBody(e packageSkillEntry) string {
	return fmt.Sprintf("Package skill: %s/%s\nPackage digest: %s\nSkill source: %q\nResource base directory: %q\nRead declared relative text with load_skill(name=%q, resource=RELATIVE_PATH); each read verifies the pinned resource hash. Other filesystem access remains subject to the active policy and execution boundary. Loading this guidance grants no execution permission.\n\n%s", e.resource.Package, e.resource.Path, e.resource.PackageDigest, e.resource.SourcePath, e.resource.BaseDirectory, e.name, e.resource.Text)
}

func (p *packageSkills) Get(name string) (string, bool) {
	if p.base != nil {
		if body, ok := p.base.Get(name); ok {
			return body, true
		}
	}
	for _, e := range p.entries {
		if e.name == name {
			return packageSkillBody(e), true
		}
	}
	return "", false
}
func (p *packageSkills) FormatIndex() string { index, _ := p.IndexSnapshot(); return index }
func (p *packageSkills) IndexSnapshot() (string, []runtime.ContextSource) {
	var text strings.Builder
	var sources []runtime.ContextSource
	if p.base != nil {
		if indexed, ok := p.base.(interface {
			IndexSnapshot() (string, []runtime.ContextSource)
		}); ok {
			index, baseSources := indexed.IndexSnapshot()
			text.WriteString(index)
			sources = append(sources, baseSources...)
		} else {
			text.WriteString(p.base.FormatIndex())
		}
	}
	for _, e := range p.entries {
		if p.base != nil {
			if _, ok := p.base.Get(e.name); ok {
				continue
			}
		}
		identity := "package:" + e.resource.Package + "@" + e.resource.PackageDigest + "/" + e.resource.Path
		line := fmt.Sprintf("\n- %s: installed package skill (source: %q; package: %s)\n", e.name, e.resource.SourcePath, identity)
		text.WriteString(line)
		sources = append(sources, runtime.ContextSource{Kind: "skill_index", Path: e.resource.SourcePath, EstimatedTokens: len(line) / 4})
	}
	return text.String(), sources
}

// SelectPackageSkills replaces the explicit package skill selection under idle
// application ownership. Bodies are immutable verified snapshots; package updates
// require a new selection. Existing workspace/personal skills retain precedence.
// An empty selection removes package skills without changing authored stores.
func (c *Controller) SelectPackageSkills(ctx context.Context, store *packages.Store, selected []PackageSkillSelection) error {
	operation, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return err
	}
	defer release()
	if len(selected) > 64 || (len(selected) > 0 && store == nil) {
		return errors.New("select at most 64 package skills from an open store")
	}
	seen := map[string]bool{}
	entries := []packageSkillEntry{}
	requests := []packages.TextResourceSelection{}
	for _, selection := range selected {
		if !skills.ValidName(selection.Name) || seen[selection.Name] {
			return errors.New("invalid or duplicate selected skill name")
		}
		seen[selection.Name] = true
		requests = append(requests, packages.TextResourceSelection{Package: selection.Package, Path: selection.Path})
	}
	if len(requests) > 0 {
		resources, err := store.ReadTextResources(operation, requests)
		if err != nil {
			return err
		}
		for i, resource := range resources {
			if resource.PackageDigest != selected[i].Digest {
				return errors.New("selected package skill differs from pinned digest")
			}
			if resource.Kind != "skill" {
				return errors.New("selected resource is not a skill")
			}
			entries = append(entries, packageSkillEntry{selected[i].Name, resource})
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if err = operation.Err(); err != nil {
		return err
	}
	if c.Rt == nil {
		return errors.New("runtime unavailable")
	}
	registry, ok := c.Rt.Tools.(*tool.Registry)
	if !ok {
		return errors.New("skill tool registry unavailable")
	}
	previousTool, ok := c.Rt.Tools.Get("load_skill")
	if !ok {
		return errors.New("skill loader unavailable")
	}
	baseTool := previousTool
	if wrapped, ok := baseTool.(*packageResourceSkillTool); ok {
		baseTool = wrapped.base
	}
	previous := c.Rt.Skills
	base := previous
	if old, ok := base.(*packageSkills); ok {
		base = old.base
	}
	if len(entries) == 0 {
		c.Rt.Skills = base
		registry.Register(baseTool)
	} else {
		c.Rt.Skills = &packageSkills{base, entries}
		registry.Register(&packageResourceSkillTool{base: baseTool, rt: c.Rt})
	}
	if err = c.Rt.RefreshSkills(); err != nil {
		c.Rt.Skills = previous
		registry.Register(previousTool)
		return err
	}
	return nil
}
