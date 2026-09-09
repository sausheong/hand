package tui

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/extensions"
)

func TestExtensionTerminalQuestionAndJoinedResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "note")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./examples/extensions/go-task-note")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	m, _, _ := sessionUIFixture(t)
	defer m.CloseApplication()
	review, err := extensions.ReviewHostLaunch(ctx, extensions.LaunchConfig{Name: "task-note", Executable: binary, Workspace: t.TempDir(), Capabilities: []string{"commands", "questions", "state", "context.transform"}})
	if err != nil {
		t.Fatal(err)
	}
	factory, err := extensions.NewHostFactory(filepath.Join(t.TempDir(), "private"), []extensions.LaunchReview{review}, func(context.Context, extensions.LaunchReview) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	host, err := app.NewExtensionHost(ctx, m.controller, factory, map[string]string{"task-note": "examples/note"})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	m.controller.Extensions = host
	if _, err = host.Reload(ctx, []extensions.Specification{review.Specification}); err != nil {
		t.Fatal(err)
	}
	command := m.runExtension([]string{"task-note", "note"})
	batch := command().(tea.BatchMsg)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for len(host.Pending()) == 0 {
		select {
		case <-ctx.Done():
			t.Fatal("question timeout")
		case <-ticker.C:
		}
	}
	m.pollExtensionQuestion(extensionQuestionTick{m.extensionGeneration})
	if m.extensionQuestion == nil || !m.sessionChanging {
		t.Fatal("question not presented under ownership")
	}
	m.textarea.SetValue("terminal note")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.extensionQuestion != nil {
		t.Fatal("answered question retained")
	}
	m.Update(batch[0]())
	if m.sessionChanging {
		t.Fatal("joined command retained ownership")
	}
	foundPresentation := false
	for _, source := range m.sourceBlocks {
		if source.Block.Kind == "extension_text" && strings.Contains(source.Block.Text, "Task note saved") {
			foundPresentation = true
		}
	}
	if !foundPresentation {
		t.Fatal("real command result missing typed presentation")
	}
	if cmd := m.pollExtensionQuestion(extensionQuestionTick{m.extensionGeneration}); cmd != nil {
		t.Fatal("poll continued after command finished")
	}
	store, err := extensions.NewStateStore(m.controller.Rt.Session, "examples/note")
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.Get(ctx)
	if err != nil || string(state.Data) != `{"note":"terminal note"}` {
		t.Fatalf("terminal state: %+v %v", state, err)
	}
}
func TestExtensionAnswerModes(t *testing.T) {
	q := extensions.PendingQuestion{Question: protocol.Question{ID: "q", Title: "Pick", Options: []protocol.Option{{ID: "yes", Label: "Yes"}}, AllowFreeText: true}}
	for _, tc := range []struct {
		text             string
		cancel           bool
		choice, textWant string
	}{{"hello", false, "", "hello"}, {"choice:yes", false, "yes", ""}, {"ignored", true, "", ""}} {
		answer := extensionAnswer(q, tc.text, tc.cancel)
		if answer.Choice != tc.choice || answer.Text != tc.textWant || answer.Cancelled != tc.cancel {
			t.Fatalf("answer %+v", answer)
		}
		if err := q.Question.ValidateAnswer(answer); err != nil {
			t.Fatal(err)
		}
	}
	q.Question.AllowFreeText = false
	if answer := extensionAnswer(q, "yes", false); answer.Choice != "yes" {
		t.Fatal(answer)
	}
}
