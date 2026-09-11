package tui

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestSkillsListHasReadableHierarchy(t *testing.T) {
	index := "## Skills\n\n- design: Make readable interfaces. (source: /tmp/design/SKILL.md)\n- code: Fix code: preserve details. (source: /tmp/code/SKILL.md)\nSkill discovery: a warning\n"
	block := TranscriptBlock{Kind: "skills", Text: index}
	got := ansi.Strip(block.render(80, ""))
	for _, want := range []string{"Skills\nScroll", "design\nMake readable interfaces.\nSource: /tmp/design/SKILL.md\n\ncode\n", "Fix code: preserve details.", "Skill discovery: a warning"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	if !userLineStyle.GetBold() {
		t.Fatal("skill names must be bold")
	}
	m := NewModel(nil, t.TempDir())
	defer m.CloseApplication()
	m.SetSkillsIndex(index)
	m.runSkillsCommand()
	if m.skillsIndex != index {
		t.Fatal("presentation changed model discovery index")
	}
}

func TestLongSkillsListingStartsAtHeadingAndScrolls(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	defer m.CloseApplication()
	m.termWidth = 80
	m.termHeight = 24
	m.SetSkillsIndex("## Skills\n" + strings.Repeat("- example: Description. (source: /tmp/example/SKILL.md)\n", 60))
	m.handleCommand("/skills")
	if !strings.Contains(ansi.Strip(m.viewport.View()), "Skills") || m.viewport.AtBottom() {
		t.Fatal("long list did not open at heading")
	}
	m.viewport.HalfPageDown()
	if m.viewport.YOffset == 0 {
		t.Fatal("long skill list cannot scroll")
	}
}

func TestReloadCommandGuardsAndJoins(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	if cmd := m.handleCommand("/reload unexpected"); cmd != nil {
		t.Fatal("invalid reload dispatched")
	}
	m.running = true
	if cmd := m.handleCommand("/reload"); cmd != nil {
		t.Fatal("busy reload dispatched")
	}
	m.running = false
	cmd := m.handleCommand("/reload")
	if cmd == nil || !m.sessionChanging {
		t.Fatal("reload did not enter joined operation")
	}
	m.Update(cmd())
	if m.sessionChanging {
		t.Fatal("reload operation did not finish")
	}
}
