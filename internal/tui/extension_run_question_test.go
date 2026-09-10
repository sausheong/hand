package tui

import (
	"context"
	"encoding/json"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/llm/llmtest"
	"github.com/sausheong/harness/tool"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunQuestionPeerProcess(t *testing.T) {
	if os.Getenv("HAND_RUN_QUESTION_PEER") == "" {
		return
	}
	reader, writer := protocol.NewReader(os.Stdin), protocol.NewWriter(os.Stdout)
	for {
		f, err := reader.Read()
		if err != nil {
			os.Exit(0)
		}
		result := json.RawMessage(`{}`)
		if f.Method == "initialize" {
			result = json.RawMessage(`{"version":1,"name":"run-question","capabilities":["lifecycle","questions"],"subscriptions":["run.start"]}`)
		} else {
			if err = writer.Write(protocol.Frame{Version: 1, Kind: "request", ID: "question", Method: "user.question", Params: json.RawMessage(`{"id":"before-run","title":"What should this run remember?","allow_free_text":true}`)}); err != nil {
				os.Exit(2)
			}
			answer, err := reader.Read()
			if err != nil || answer.Error != nil {
				os.Exit(3)
			}
		}
		if err = writer.Write(protocol.Frame{Version: 1, Kind: "response", ID: f.ID, Result: result}); err != nil {
			os.Exit(4)
		}
	}
}

type runQuestionProvider struct{ llmtest.Base }

func (*runQuestionProvider) ChatStream(context.Context, llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	out := make(chan llm.ChatEvent, 1)
	out <- llm.ChatEvent{Type: llm.EventDone}
	close(out)
	return out, nil
}
func TestModelRunExtensionQuestionPreservesDraftAndGeneration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	m, _, _ := sessionUIFixture(t)
	defer m.CloseApplication()
	rt := m.controller.Rt
	rt.LLM = &runQuestionProvider{}
	rt.Tools = tool.NewRegistry()
	workspace := t.TempDir()
	review, err := extensions.ReviewHostLaunch(ctx, extensions.LaunchConfig{Name: "run-question", Executable: exe, Workspace: workspace, Arguments: []string{"-test.run=^TestRunQuestionPeerProcess$"}, Environment: map[string]string{"HAND_RUN_QUESTION_PEER": "1"}, Capabilities: []string{"lifecycle", "questions"}, Mandatory: true})
	if err != nil {
		t.Fatal(err)
	}
	host, err := m.controller.ActivateExtensions(ctx, app.ExtensionStartup{Version: 1, SnapshotRoot: filepath.Join(t.TempDir(), "private"), Identities: map[string]string{"run-question": "fixture/questions"}, Reviews: []extensions.LaunchReview{review}}, workspace, true)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	m.controller.Extensions = host
	m.running = true
	m.textarea.SetValue("saved follow-up draft")
	if m.startExtensionPolling() == nil {
		t.Fatal("model run did not start question polling")
	}
	done := make(chan error, 1)
	go func() {
		result, err := rt.RunTurn(ctx, "hello", nil, nil)
		if err == nil {
			err = result.Err
		}
		done <- err
	}()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for len(host.Pending()) == 0 {
		select {
		case <-ctx.Done():
			t.Fatal("question timeout")
		case <-ticker.C:
		}
	}
	if cmd := m.pollExtensionQuestion(extensionQuestionTick{m.extensionGeneration - 1}); cmd != nil || m.extensionQuestion != nil {
		t.Fatal("stale poll changed question")
	}
	m.pollExtensionQuestion(extensionQuestionTick{m.extensionGeneration})
	if m.extensionQuestion == nil || m.textarea.Value() != "" {
		t.Fatal("question did not isolate draft")
	}
	m.textarea.SetValue("run answer")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.textarea.Value() != "saved follow-up draft" {
		t.Fatal("draft not restored")
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	old := m.extensionGeneration
	m.running = false
	m.finishGoal(agentio.RunOutcome{Status: agentio.Completed})
	if cmd := m.pollExtensionQuestion(extensionQuestionTick{old}); cmd != nil {
		t.Fatal("old run polling continued")
	}
}
