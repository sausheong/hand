package tui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/compaction"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
)

func TestProfileMetadataFailurePreservesUIAndCompaction(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if fail.Load() {
			http.Error(w, "fixture unavailable", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{"default_generation_settings":{"n_ctx":24576},"n_ctx_train":262144}`)
	}))
	defer server.Close()
	old := &profileTestProvider{}
	rt := &runtime.Runtime{Provider: "local", Model: "old", LLM: old, ContextWindow: 8192, Compaction: &compaction.Manager{Summarizer: &compaction.Summarizer{Provider: old, Model: "old"}}}
	c := &Controller{Rt: rt}
	if err := c.ConfigureProfiles(map[string]config.ModelProfile{"new": {Provider: "local", Model: "new", Endpoint: server.URL + "/v1", MetadataURL: server.URL + "/props", MetadataProtocol: "llamacpp_props"}}, ""); err != nil {
		t.Fatal(err)
	}
	built := 0
	next := &profileTestProvider{}
	c.BuildProfileProvider = func(config.ModelProfile) (llm.LLMProvider, error) { built++; return next, nil }
	m := NewModel(nil, t.TempDir())
	m.SetController(c)
	m.setModel("local/old")
	m.SetContextLimit(8192)
	m.lastRequestUsage = &llm.Usage{InputTokens: 4096}
	before := m.contextSummary()
	driveApplication(t, m, m.handleCommand("/profile new"))
	if built != 0 || calls.Load() != 1 || c.CurrentModel() != "local/old" || rt.LLM != old || rt.Compaction.Summarizer.Provider != old || rt.ContextWindow != 8192 || m.model != "local/old" || m.contextSummary() != before || m.profileChanging {
		t.Fatal("failed metadata changed active profile or failed to release operation")
	}
	fail.Store(false)
	driveApplication(t, m, m.handleCommand("/profile new"))
	if built != 1 || calls.Load() != 2 || c.CurrentProfile() != "new" || c.CurrentModel() != "local/new" || rt.LLM != next || rt.Compaction.Summarizer.Provider != next || rt.Compaction.Summarizer.Model != "new" || rt.ContextWindow != 24576 || m.contextWindow != 24576 || m.lastRequestUsage != nil || m.model != "local/new" || m.profileChanging {
		t.Fatal("successful metadata switch did not atomically update model, context and compaction")
	}
}
