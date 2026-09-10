package app

import (
	"context"
	"errors"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/compaction"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"strings"
	"testing"
	"time"
)

func TestSeparateSummarizerReviewAtomicityAndModelSwitch(t *testing.T) {
	ctx := context.Background()
	mainProvider := &focusProvider{}
	summaryProvider := &focusProvider{}
	original := &compaction.Summarizer{Provider: mainProvider, Model: "main"}
	c := &Controller{Rt: &runtime.Runtime{LLM: mainProvider, Provider: "local", Model: "main", Session: session.NewSession("hand", "summary"), Compaction: &compaction.Manager{Summarizer: original}}}
	profiles := map[string]config.ModelProfile{"summary": {Provider: "openai", Model: "summary-model", Endpoint: "https://summary.example/v1", CredentialEnv: "SUMMARY_KEY"}, "main": {Provider: "local", Model: "main"}, "next": {Provider: "local", Model: "next"}}
	if err := c.ConfigureProfiles(profiles, "main"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	factoryError := false
	c.BuildProfileProvider = func(p config.ModelProfile) (llm.LLMProvider, error) {
		calls++
		if factoryError {
			return nil, errors.New("fixture factory failed")
		}
		if p.Provider == "openai" {
			if p.Endpoint != "https://summary.example/v1" || p.CredentialEnv != "SUMMARY_KEY" {
				t.Fatal("route not preserved")
			}
			return summaryProvider, nil
		}
		return mainProvider, nil
	}
	options := SummarizerOptions{Profile: "summary", MaxOutputTokens: 1024, TimeoutSeconds: 30}
	review, err := c.ReviewSummarizer(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 || !strings.Contains(review.Disclosure, "session history") || review.Destination != "https://summary.example/v1" || review.CredentialReference != "SUMMARY_KEY" {
		t.Fatalf("review %+v", review)
	}
	if err = c.SelectSummarizer(ctx, options, "wrong"); err == nil || calls != 0 {
		t.Fatal("unreviewed selection constructed provider")
	}
	factoryError = true
	if err = c.SelectSummarizer(ctx, options, review.Digest); err == nil || c.Rt.Compaction.Summarizer != original {
		t.Fatal("failed selection changed configuration")
	}
	factoryError = false
	if err = c.SelectSummarizer(ctx, options, review.Digest); err != nil {
		t.Fatal(err)
	}
	selected := c.Rt.Compaction.Summarizer
	if selected.Route.Provider != "openai" || selected.Route.Destination != "https://summary.example/v1" || selected.Provider != summaryProvider || selected.MaxOutputTokens != 1024 || selected.Timeout != 30*time.Second {
		t.Fatal("selected configuration mismatch")
	}
	if err = c.SwitchModel("local/new-main"); err != nil {
		t.Fatal(err)
	}
	if err = c.SwitchProfile("next"); err != nil {
		t.Fatal(err)
	}
	if c.Rt.Compaction.Summarizer != selected || selected.Provider != summaryProvider || selected.Model != "summary-model" {
		t.Fatal("main switch overwrote independent summariser")
	}
	saved, ok := c.SelectedSummarizer()
	if !ok || saved.Digest != review.Digest {
		t.Fatal("selection disclosure lost")
	}
	saved.Model = "caller mutation"
	again, _ := c.SelectedSummarizer()
	if again.Model != review.Model {
		t.Fatal("mutable view shared")
	}
	_, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		t.Fatal(err)
	}
	err = c.SelectSummarizer(ctx, options, review.Digest)
	release()
	if !errors.Is(err, ErrBusy) {
		t.Fatal("busy selection accepted")
	}
	follow := SummarizerOptions{FollowMain: true, MaxOutputTokens: 512, TimeoutSeconds: 20}
	following, err := c.ReviewSummarizer(ctx, follow)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(following.Disclosure, "follows subsequent") || following.Model != "next" {
		t.Fatalf("follow review %+v", following)
	}
	if err = c.SwitchModel("local/changed-main"); err != nil {
		t.Fatal(err)
	}
	if err = c.SelectSummarizer(ctx, follow, following.Digest); err == nil || c.Rt.Compaction.Summarizer != selected {
		t.Fatal("stale follow review changed destination")
	}
	following, err = c.ReviewSummarizer(ctx, follow)
	if err != nil {
		t.Fatal(err)
	}
	beforeCalls := calls
	if err = c.SelectSummarizer(ctx, follow, following.Digest); err != nil {
		t.Fatal(err)
	}
	if c.Rt.Compaction.Summarizer.Route != c.Rt.Route || calls != beforeCalls || c.Rt.Compaction.Summarizer.Provider != c.Rt.LLM {
		t.Fatal("follow rebuilt provider or used wrong destination")
	}
	if err = c.SwitchModel("local/final-main"); err != nil {
		t.Fatal(err)
	}
	effective, independent := c.SelectedSummarizer()
	if independent || !effective.Options.FollowMain || effective.Model != "final-main" || c.Rt.Compaction.Summarizer.Model != "final-main" || effective.Options.MaxOutputTokens != 512 || effective.Options.TimeoutSeconds != 20 {
		t.Fatalf("follow state %+v", effective)
	}

}
