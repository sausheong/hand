package agentio_test

import (
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/llm"
)

func TestBuildCompactionManager_SetsSummarizerFields(t *testing.T) {
	var fakeProvider llm.LLMProvider = struct{ llm.LLMProvider }{}

	mgr := agentio.BuildCompactionManager(fakeProvider, "claude-sonnet-5")

	if mgr == nil {
		t.Fatal("BuildCompactionManager returned nil")
	}
	if mgr.Summarizer == nil {
		t.Fatal("Summarizer is nil")
	}
	if mgr.Summarizer.Provider != fakeProvider {
		t.Errorf("Summarizer.Provider = %v, want %v", mgr.Summarizer.Provider, fakeProvider)
	}
	if mgr.Summarizer.Model != "claude-sonnet-5" {
		t.Errorf("Summarizer.Model = %q, want %q", mgr.Summarizer.Model, "claude-sonnet-5")
	}
}
