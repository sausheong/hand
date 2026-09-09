package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/llm/llmtest"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool/skills/disk"
)

type approvalSkillProvider struct {
	llmtest.Base
	calls int
}

func (p *approvalSkillProvider) ChatStream(context.Context, llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	p.calls++
	events := make(chan llm.ChatEvent, 2)
	if p.calls == 1 {
		events <- llm.ChatEvent{Type: llm.EventToolCallDone, ToolCall: &llm.ToolCall{ID: "create", Name: "skill_manage", Input: json.RawMessage(`{"action":"create","name":"new-skill","body":"approved body"}`)}}
	}
	events <- llm.ChatEvent{Type: llm.EventDone}
	close(events)
	return events, nil
}
func TestScopedSkillMutationWaitsForTUIApproval(t *testing.T) {
	for _, decision := range []string{"allow", "deny", "cancel"} {
		t.Run(decision, func(t *testing.T) {
			workspace := t.TempDir()
			root := filepath.Join(workspace, ".hand", "skills")
			path := filepath.Join(root, "new-skill", "SKILL.md")
			provider := &approvalSkillProvider{}
			sess := session.NewSession("hand", "approval")
			defer sess.Close()
			rt := &runtime.Runtime{LLM: provider, Tools: agentio.BuildRegistry(workspace, disk.NewStore(root)), Session: sess, Model: "fixture", MaxTurns: 2, AgentLoop: runtime.LoopConfig{Hooks: runtime.LifecycleHooks{BeforeToolUse: app.NewScopedApprovalHook(nil, workspace, "fixture", nil)}}}
			service := app.New(&app.HarnessBackend{Runtime: rt}, app.Options{SessionID: sess.ID, MaxIterations: 1})
			m := applicationModel(t, service, workspace)
			defer m.CloseApplication()
			cmd := m.startRun("create skill")
			timeout := time.After(3 * time.Second)
			for m.pending == nil {
				if cmd == nil {
					t.Fatal("execution ended before approval")
				}
				result := make(chan tea.Msg, 1)
				go func(next tea.Cmd) { result <- next() }(cmd)
				select {
				case msg := <-result:
					_, cmd = m.Update(msg)
				case <-timeout:
					t.Fatal("approval not delivered")
				}
			}
			if m.pendingApprovalID == "" || m.pending.Tool != "skill_manage" || service.Snapshot().State != app.AwaitingApproval {
				t.Fatal("scoped decision not application-owned")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("skill written before decision", err)
			}
			key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}
			if decision == "allow" {
				key.Runes = []rune{'y'}
			} else if decision == "cancel" {
				key = tea.KeyMsg{Type: tea.KeyCtrlC}
			}
			m.Update(key)
			driveApplication(t, m, cmd)
			if m.pending != nil || m.pendingApprovalID != "" || m.running || service.Snapshot().State != app.Idle {
				t.Fatal("decision retained pending work")
			}
			data, err := os.ReadFile(path)
			if decision == "allow" {
				if err != nil || len(data) == 0 {
					t.Fatal("approval did not create skill", err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatal("denied/cancelled mutation wrote skill", err)
			}
			expected := agentio.Completed
			if decision == "cancel" {
				expected = agentio.Cancelled
			}
			if m.lastOutcome == nil || m.lastOutcome.Status != expected {
				t.Fatal("incorrect terminal outcome", m.lastOutcome)
			}
		})
	}
}
