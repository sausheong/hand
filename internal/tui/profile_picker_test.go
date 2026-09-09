package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
)

func TestProfilePickerSelectionCancelAndBounds(t *testing.T) {
	c := &Controller{Rt: &runtime.Runtime{Provider: "local", Model: "old"}}
	profiles := map[string]config.ModelProfile{}
	for i := 0; i < 30; i++ {
		profiles[fmt.Sprintf("profile-%02d", i)] = config.ModelProfile{Provider: "local", Model: "model", ContextLimit: 16384, MaxOutput: 1024}
	}
	profiles["two words"] = config.ModelProfile{Provider: "local", Model: "selected", ContextLimit: 24576, MaxOutput: 512}
	if err := c.ConfigureProfiles(profiles, ""); err != nil {
		t.Fatal(err)
	}
	calls := 0
	c.BuildProfileProvider = func(config.ModelProfile) (llm.LLMProvider, error) { calls++; return &profileTestProvider{}, nil }
	m := NewModel(nil, t.TempDir())
	m.SetController(c)
	m.setModel("local/old")
	m.textarea.SetValue("preserved draft")
	m.goalChecking = true
	generation := m.goalGeneration
	m.handleCommand("/profile")
	if m.profilePicker == nil || calls != 0 || m.goalChecking || m.goalGeneration == generation {
		t.Fatal("picker built provider before selection")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyEnd})
	for _, size := range [][2]int{{80, 24}, {30, 12}, {12, 4}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		lines := strings.Split(m.View(), "\n")
		if len(lines) > size[1] {
			t.Fatal("picker exceeded terminal height")
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("picker exceeded terminal width")
			}
		}
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	if m.profilePicker != nil || calls != 0 || m.textarea.Value() != "preserved draft" || c.CurrentModel() != "local/old" {
		t.Fatal("dismissal changed model or draft")
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.handleCommand("/profile")
	m.handleKey(tea.KeyMsg{Type: tea.KeyEnd})
	if !strings.Contains(m.View(), "local/selected") || !strings.Contains(m.View(), "24576") || !strings.Contains(m.View(), "512") {
		t.Fatal("configuration preview missing", m.View())
	}
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	driveApplication(t, m, cmd)
	if calls != 1 || c.CurrentProfile() != "two words" || m.model != "local/selected" || m.profilePicker != nil {
		t.Fatal("selection did not commit exact profile name")
	}
	m.handleCommand("/profile")
	m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	if m.profilePicker.selected != 0 {
		t.Fatal("down did not wrap")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	if m.profilePicker.selected != 30 {
		t.Fatal("up did not wrap")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if m.profilePicker != nil || calls != 1 {
		t.Fatal("cancel selected a provider")
	}
}
