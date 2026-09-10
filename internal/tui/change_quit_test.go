package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
)

// Exercise keyboard dispatch, not handleCommand directly: busy guards must
// allow exit commands, retain ownership until join, and then actually quit.
func TestChangingKeyboardQuitJoinsBeforeExit(t *testing.T) {
	for _, phase := range []string{"session", "profile"} {
		for _, command := range []string{"/quit", "/exit"} {
			t.Run(phase+command, func(t *testing.T) {
				entered, release := make(chan struct{}), make(chan struct{})
				var m *Model
				var cmd tea.Cmd
				if phase == "session" {
					m, _, _ = sessionUIFixture(t)
					cmd = m.startSessionChange("resume", func(ctx context.Context) error {
						close(entered)
						<-release
						return ctx.Err()
					})
				} else {
					c := &Controller{Rt: &runtime.Runtime{Provider: "local", Model: "old"}}
					if err := c.ConfigureProfiles(map[string]config.ModelProfile{"next": {Provider: "local", Model: "next"}}, ""); err != nil {
						t.Fatal(err)
					}
					c.BuildProfileProvider = func(config.ModelProfile) (llm.LLMProvider, error) {
						close(entered)
						<-release
						return &profileTestProvider{}, nil
					}
					m = NewModel(nil, t.TempDir())
					m.SetController(c)
					m.setModel("local/old")
					cmd = m.handleCommand("/profile next")
				}
				released := false
				defer func() {
					if !released {
						close(release)
					}
					m.CloseApplication()
				}()
				select {
				case <-entered:
				case <-time.After(time.Second):
					t.Fatal("change did not start")
				}
				identity, model := m.identity, m.model
				m.textarea.SetValue(command)
				_, immediate := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
				if immediate != nil {
					t.Fatal("quit escaped before worker joined")
				}
				if phase == "session" && (!m.quitAfterSession || !m.sessionChanging) {
					t.Fatal("keyboard quit did not retain pending session shutdown")
				}
				if phase == "profile" && (!m.quitAfterProfile || !m.profileChanging) {
					t.Fatal("keyboard quit did not retain pending profile shutdown")
				}
				m.textarea.SetValue("must not start")
				m.Update(tea.KeyMsg{Type: tea.KeyEnter})
				if m.running {
					t.Fatal("new prompt raced exiting worker")
				}
				close(release)
				released = true
				_, quit := m.Update(cmd())
				if quit == nil {
					t.Fatal("joined worker did not finish quit")
				}
				if _, ok := quit().(tea.QuitMsg); !ok {
					t.Fatal("expected actual quit message")
				}
				if m.sessionChanging || m.profileChanging || m.identity != identity || m.model != model {
					t.Fatal("cancelled change mutated UI or retained guard")
				}
				if phase == "profile" && m.controller.CurrentModel() != "local/old" {
					t.Fatal("cancelled provider preparation committed")
				}
			})
		}
	}
}
