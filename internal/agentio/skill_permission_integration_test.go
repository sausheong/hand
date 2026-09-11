package agentio_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/llm/llmtest"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool/skills"
	"github.com/sausheong/harness/tool/skills/disk"
)

type skillMutationProvider struct {
	llmtest.Base
	input json.RawMessage
	calls int
}

func (p *skillMutationProvider) ChatStream(context.Context, llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	p.calls++
	events := make(chan llm.ChatEvent, 2)
	if p.calls == 1 {
		events <- llm.ChatEvent{Type: llm.EventToolCallDone, ToolCall: &llm.ToolCall{ID: "mutation", Name: "skill_manage", Input: p.input}}
	}
	if p.calls > 1 {
		events <- llm.ChatEvent{Type: llm.EventTextDelta, Text: "Answer"}
	}
	events <- llm.ChatEvent{Type: llm.EventDone}
	close(events)
	return events, nil
}
func TestRuntimeSkillDenialPreservesDiskInBothApprovalModes(t *testing.T) {
	for _, mode := range []string{"oneshot", "interactive"} {
		for _, action := range []string{"create", "patch", "replace", "remove"} {
			t.Run(mode+"/"+action, func(t *testing.T) {
				dir := t.TempDir()
				store := disk.NewStore(dir)
				if _, err := store.Create(context.Background(), skills.Skill{Name: "existing", Body: "original body"}); err != nil {
					t.Fatal(err)
				}
				originalPath := filepath.Join(dir, "existing", "SKILL.md")
				original, err := os.ReadFile(originalPath)
				if err != nil {
					t.Fatal(err)
				}
				name := "existing"
				if action == "create" {
					name = "new-skill"
				}
				input, _ := json.Marshal(map[string]string{"action": action, "name": name, "body": "replacement body", "old_string": "original", "new_string": "changed"})
				reg := agentio.BuildRegistry(t.TempDir(), store)
				provider := &skillMutationProvider{input: input}
				hook := agentio.NewOneShotApprovalHook(nil, false, nil, nil)
				sender := &decidingSender{decision: agentio.DecisionDeny}
				if mode == "interactive" {
					hook = agentio.NewApprovalHook(sender, nil, t.TempDir(), nil, nil)
				}
				sess := session.NewSession("hand", "key")
				defer sess.Close()
				rt := &runtime.Runtime{LLM: provider, Tools: reg, Session: sess, Model: "fixture", MaxTurns: 2, AgentLoop: runtime.LoopConfig{Hooks: runtime.LifecycleHooks{BeforeToolUse: hook}}}
				events, err := rt.Run(context.Background(), "go", nil)
				if err != nil {
					t.Fatal(err)
				}
				denied := 0
				for event := range events {
					if event.Type == runtime.EventToolResult && event.Result != nil && event.Result.Error != "" {
						denied++
					}
				}
				if provider.calls != 2 || denied != 1 || (mode == "interactive" && sender.prompts != 1) {
					t.Fatal("runtime did not expose one denied tool", provider.calls, denied, sender.prompts)
				}
				after, err := os.ReadFile(originalPath)
				if err != nil || string(after) != string(original) {
					t.Fatal("denied mutation changed existing skill", err)
				}
				if _, err := os.Stat(filepath.Join(dir, "new-skill")); !os.IsNotExist(err) {
					t.Fatal("denied create wrote files", err)
				}
				// Prove the denied input was a valid mutation, not an invalid no-op.
				result, err := reg.Execute(context.Background(), "skill_manage", input)
				if err != nil || !strings.Contains(result.Output, `"success":true`) {
					t.Fatal("mutation fixture was invalid", result, err)
				}
				if action == "remove" {
					if _, err := os.Stat(originalPath); !os.IsNotExist(err) {
						t.Fatal("permitted remove did not remove skill", err)
					}
				} else {
					target := originalPath
					if action == "create" {
						target = filepath.Join(dir, "new-skill", "SKILL.md")
					}
					changed, err := os.ReadFile(target)
					if err != nil || string(changed) == string(original) {
						t.Fatal("permitted mutation had no effect", err)
					}
				}
			})
		}
	}
}
