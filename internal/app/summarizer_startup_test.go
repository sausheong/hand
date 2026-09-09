package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/compaction"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
)

func TestSummarizerStartupStrictFile(t *testing.T) {
	valid := `{"version":1,"options":{"profile":"summary"},"digest":"` + strings.Repeat("a", 64) + `"}`
	for name, body := range map[string]string{
		"valid":          valid,
		"follow":         strings.Replace(valid, `"profile":"summary"`, `"follow_main":true`, 1),
		"unknown":        strings.Replace(valid, `"version":1`, `"version":1,"secret":"value"`, 1),
		"unknown_option": strings.Replace(valid, `"profile":"summary"`, `"profile":"summary","secret":true`, 1),
		"version":        strings.Replace(valid, `"version":1`, `"version":2`, 1),
		"digest":         strings.Replace(valid, strings.Repeat("a", 64), "wrong", 1),
		"both":           strings.Replace(valid, `"profile":"summary"`, `"profile":"summary","follow_main":true`, 1),
		"neither":        strings.Replace(valid, `"profile":"summary"`, `"profile":""`, 1),
		"negative":       strings.Replace(valid, `"profile":"summary"`, `"profile":"summary","timeout_seconds":-1`, 1),
		"tokens":         strings.Replace(valid, `"profile":"summary"`, `"profile":"summary","max_output_tokens":32769`, 1),
		"trailing":       valid + ` {}`, "oversize": valid + strings.Repeat(" ", 16<<10),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "selection.json")
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := ReadSummarizerStartup(path)
			wantOK := name == "valid" || name == "follow"
			if (err == nil) != wantOK {
				t.Fatalf("accepted=%v, want %v: %v", err == nil, wantOK, err)
			}
		})
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "regular")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte(valid), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dir, link, filepath.Join(dir, "missing")} {
		if _, err := ReadSummarizerStartup(path); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
}

func TestSummarizerStartupRestartRevalidatesDestination(t *testing.T) {
	ctx := context.Background()
	makeController := func(endpoint string, calls *int) *Controller {
		provider := &focusProvider{}
		c := &Controller{Rt: &runtime.Runtime{LLM: provider, Provider: "local", Model: "main", Compaction: &compaction.Manager{Summarizer: &compaction.Summarizer{Provider: provider, Model: "main"}}}}
		if err := c.ConfigureProfiles(map[string]config.ModelProfile{"summary": {Provider: "local", Model: "summary", Endpoint: endpoint}}, ""); err != nil {
			t.Fatal(err)
		}
		c.BuildProfileProvider = func(config.ModelProfile) (llm.LLMProvider, error) { *calls++; return provider, nil }
		return c
	}
	calls := 0
	first := makeController("http://localhost:1234/v1", &calls)
	review, err := first.ReviewSummarizer(ctx, SummarizerOptions{Profile: "summary", MaxOutputTokens: 512, TimeoutSeconds: 20})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(SummarizerStartup{Version: 1, Options: review.Options, Digest: review.Digest})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "selection.json")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	saved, err := ReadSummarizerStartup(path)
	if err != nil {
		t.Fatal(err)
	}
	restarted := makeController("http://localhost:1234/v1", &calls)
	if err = restarted.SelectSummarizer(ctx, saved.Options, saved.Digest); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || restarted.Rt.Compaction.Summarizer.Model != "summary" || restarted.Rt.Compaction.Summarizer.MaxOutputTokens != 512 {
		t.Fatal("restart did not restore reviewed route and limits")
	}
	changed := makeController("http://localhost:5678/v1", &calls)
	original := changed.Rt.Compaction.Summarizer
	if err = changed.SelectSummarizer(ctx, saved.Options, saved.Digest); err == nil {
		t.Fatal("stale destination accepted")
	}
	if calls != 1 || changed.Rt.Compaction.Summarizer != original {
		t.Fatal("stale review constructed provider or changed configuration")
	}
}
