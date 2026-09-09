package tui

import (
	"github.com/sausheong/hand/internal/app"
	"strings"
	"testing"
)

func TestSummarizerReviewDisclosureAndSingleUse(t *testing.T) {
	view := app.SummarizerView{Options: app.SummarizerOptions{Profile: "summary", MaxOutputTokens: 1024, TimeoutSeconds: 30}, Destination: "https://summary.example/v1", CredentialReference: "SUMMARY_KEY", Disclosure: "Session history goes to this destination", Digest: "digest"}
	text := strings.Join(summarizerLines(view), "\n")
	for _, want := range []string{view.Destination, "SUMMARY_KEY", "1024 output tokens", "30 seconds", view.Disclosure} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
	m, _, _ := sessionUIFixture(t)
	if cmd := m.handleCommand("/summarizer summary invalid 30"); cmd != nil {
		t.Fatal("invalid limits dispatched")
	}
	if cmd := m.handleCommand("/summarizer-confirm digest"); cmd != nil {
		t.Fatal("missing review dispatched")
	}
	m.summarizerReview = &view
	m.running = true
	if cmd := m.handleCommand("/summarizer-confirm digest"); cmd != nil {
		t.Fatal("busy selection dispatched")
	}
	m.running = false
	cmd := m.handleCommand("/summarizer-confirm digest")
	if cmd == nil || m.summarizerReview != nil {
		t.Fatal("review not consumed")
	}
	m.Update(cmd())
	if m.sessionChanging || m.summarizerReview != nil {
		t.Fatal("failed operation retained reusable review")
	}
}
